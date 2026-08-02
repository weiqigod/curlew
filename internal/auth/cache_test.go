package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestObfuscate(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		profileName string
	}{
		{"roundtrip simple value", "tok123", "myprofile"},
		{"roundtrip empty value", "", "myprofile"},
		{"roundtrip special chars", "abc!@#$%^&*()\nfoo", "myprofile"},
		{"roundtrip long value", "this is a longer secret token value that exceeds 32 bytes", "myprofile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := Obfuscate(tt.value, tt.profileName)
			// Must differ from plaintext (except empty)
			if tt.value != "" && encoded == tt.value {
				t.Errorf("Obfuscate(%q) = %q, want different from input", tt.value, encoded)
			}
			got, err := Deobfuscate(encoded, tt.profileName)
			if err != nil {
				t.Fatalf("Deobfuscate returned error: %v", err)
			}
			if got != tt.value {
				t.Errorf("roundtrip: got %q, want %q", got, tt.value)
			}
		})
	}

	t.Run("different values produce different output", func(t *testing.T) {
		a := Obfuscate("secretA", "myprofile")
		b := Obfuscate("secretB", "myprofile")
		if a == b {
			t.Error("different values produced identical obfuscated output")
		}
	})

	t.Run("different profiles produce different output", func(t *testing.T) {
		a := Obfuscate("secret", "profile1")
		b := Obfuscate("secret", "profile2")
		if a == b {
			t.Error("different profiles produced identical obfuscated output")
		}
	})
}

func TestDeobfuscateErrors(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{"invalid base64", "not-valid-base64!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Deobfuscate(tt.encoded, "myprofile")
			if !errors.Is(err, ErrCacheCorrupted) {
				t.Errorf("Deobfuscate(%q) error = %v, want ErrCacheCorrupted", tt.encoded, err)
			}
		})
	}
}

func TestFileCacheStore(t *testing.T) {
	makeEntry := func(profile, collection string, expiresAt time.Time, vars map[string]string) *CacheEntry {
		obf := make(map[string]string, len(vars))
		for k, v := range vars {
			obf[k] = Obfuscate(v, profile)
		}
		return &CacheEntry{
			ProfileName: profile,
			Variables:   obf,
			ExpiresAt:   expiresAt,
			CreatedAt:   time.Now(),
			Collection:  collection,
		}
	}

	t.Run("save and load roundtrip", func(t *testing.T) {
		dir := t.TempDir()
		s := NewFileCacheStore(dir)
		entry := makeEntry("prof", "auth.yaml", time.Now().Add(time.Hour), map[string]string{"token": "abc"})
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := s.Load("prof")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		val, deErr := Deobfuscate(got.Variables["token"], "prof")
		if deErr != nil {
			t.Fatalf("Deobfuscate: %v", deErr)
		}
		if val != "abc" {
			t.Errorf("token = %q, want %q", val, "abc")
		}
	})

	t.Run("load missing file returns ErrCacheMiss", func(t *testing.T) {
		s := NewFileCacheStore(t.TempDir())
		_, err := s.Load("nonexistent")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("got %v, want ErrCacheMiss", err)
		}
	})

	t.Run("load expired entry returns ErrCacheExpired", func(t *testing.T) {
		dir := t.TempDir()
		s := NewFileCacheStore(dir)
		entry := makeEntry("prof", "auth.yaml", time.Now().Add(-time.Second), map[string]string{"token": "old"})
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save: %v", err)
		}
		_, err := s.Load("prof")
		if !errors.Is(err, ErrCacheExpired) {
			t.Errorf("got %v, want ErrCacheExpired", err)
		}
	})

	t.Run("load corrupted file returns ErrCacheCorrupted", func(t *testing.T) {
		dir := t.TempDir()
		s := NewFileCacheStore(dir)
		cacheDir := filepath.Join(dir, ".curlew", "cache")
		if err := os.MkdirAll(cacheDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cacheDir, "auth_prof.json"), []byte("not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := s.Load("prof")
		if !errors.Is(err, ErrCacheCorrupted) {
			t.Errorf("got %v, want ErrCacheCorrupted", err)
		}
	})

	t.Run("invalidate removes file", func(t *testing.T) {
		dir := t.TempDir()
		s := NewFileCacheStore(dir)
		entry := makeEntry("prof", "auth.yaml", time.Now().Add(time.Hour), map[string]string{"token": "abc"})
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if err := s.Invalidate("prof"); err != nil {
			t.Fatalf("Invalidate: %v", err)
		}
		_, err := s.Load("prof")
		if !errors.Is(err, ErrCacheMiss) {
			t.Errorf("after invalidate got %v, want ErrCacheMiss", err)
		}
	})

	t.Run("invalidate nonexistent is no-op", func(t *testing.T) {
		s := NewFileCacheStore(t.TempDir())
		if err := s.Invalidate("nonexistent"); err != nil {
			t.Errorf("Invalidate nonexistent: got %v, want nil", err)
		}
	})

	t.Run("save creates cache directory", func(t *testing.T) {
		dir := t.TempDir()
		nested := filepath.Join(dir, "new_project")
		s := NewFileCacheStore(nested)
		entry := makeEntry("prof", "auth.yaml", time.Now().Add(time.Hour), map[string]string{"token": "abc"})
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save in new dir: %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(nested, ".curlew", "cache")); statErr != nil {
			t.Errorf("cache directory not created: %v", statErr)
		}
	})

	t.Run("save fails when cache dir is a file", func(t *testing.T) {
		dir := t.TempDir()
		// Create .curlew/ directory and a regular FILE named "cache" to block MkdirAll.
		curlewDir := filepath.Join(dir, ".curlew")
		if err := os.MkdirAll(curlewDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(curlewDir, "cache"), []byte("not a dir"), 0o600); err != nil {
			t.Fatal(err)
		}
		s := NewFileCacheStore(dir)
		entry := makeEntry("prof", "auth.yaml", time.Now().Add(time.Hour), map[string]string{"token": "abc"})
		if err := s.Save(entry); err == nil {
			t.Error("Save: expected error when cache dir is a file, got nil")
		}
	})

	t.Run("load preserves collection field for caller comparison", func(t *testing.T) {
		dir := t.TempDir()
		s := NewFileCacheStore(dir)
		entry := makeEntry("prof", "auth_a.yaml", time.Now().Add(time.Hour), map[string]string{"token": "abc"})
		if err := s.Save(entry); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := s.Load("prof")
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		// Collection mismatch is checked by the caller, not Load itself.
		// Verify the collection field is preserved so callers can compare.
		if got.Collection != "auth_a.yaml" {
			t.Errorf("Collection = %q, want %q", got.Collection, "auth_a.yaml")
		}
	})
}

func TestNopCacheStore(t *testing.T) {
	s := NopCacheStore{}

	_, err := s.Load("any")
	if !errors.Is(err, ErrCacheMiss) {
		t.Errorf("Load: got %v, want ErrCacheMiss", err)
	}

	if err := s.Save(&CacheEntry{}); err != nil {
		t.Errorf("Save: got %v, want nil", err)
	}

	if err := s.Invalidate("any"); err != nil {
		t.Errorf("Invalidate: got %v, want nil", err)
	}
}
