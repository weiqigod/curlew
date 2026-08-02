package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sentinel errors for cache operations.
var (
	// ErrCacheExpired is returned when a cache entry exists but its TTL has elapsed.
	ErrCacheExpired = errors.New("auth cache entry expired")
	// ErrCacheMiss is returned when no cache entry exists for the requested profile.
	ErrCacheMiss = errors.New("auth cache entry not found")
	// ErrCacheCorrupted is returned when a cache file cannot be parsed or decoded.
	ErrCacheCorrupted = errors.New("auth cache entry corrupted")
)

// CacheEntry represents cached auth profile credentials persisted to disk.
type CacheEntry struct {
	ProfileName string            `json:"profile_name"`
	Variables   map[string]string `json:"variables"` // obfuscated values
	ExpiresAt   time.Time         `json:"expires_at"`
	CreatedAt   time.Time         `json:"created_at"`
	Collection  string            `json:"collection"` // detect config drift
}

// CacheStore defines the interface for auth profile credential caching.
type CacheStore interface {
	// Load retrieves a cached entry. Returns ErrCacheMiss if the entry does not
	// exist, ErrCacheExpired if the TTL has elapsed, or ErrCacheCorrupted if the
	// file cannot be parsed. Callers treat all errors as a cache miss.
	Load(profileName string) (*CacheEntry, error)
	// Save persists a cache entry. Values should be obfuscated before calling.
	Save(entry *CacheEntry) error
	// Invalidate removes the entry. Returns nil if the entry does not exist.
	Invalidate(profileName string) error
}

// FileCacheStore is a CacheStore backed by JSON files in .apitest/cache/.
type FileCacheStore struct {
	dir string
}

// NewFileCacheStore creates a FileCacheStore rooted at projectRoot/.apitest/cache/.
func NewFileCacheStore(projectRoot string) *FileCacheStore {
	return &FileCacheStore{dir: filepath.Join(projectRoot, ".apitest", "cache")}
}

// Load retrieves a cached entry by profile name.
func (s *FileCacheStore) Load(profileName string) (*CacheEntry, error) {
	data, err := os.ReadFile(s.path(profileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCacheCorrupted, err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		_ = os.Remove(s.path(profileName))
		return nil, fmt.Errorf("%w: %w", ErrCacheCorrupted, err)
	}

	if time.Now().After(entry.ExpiresAt) {
		_ = os.Remove(s.path(profileName))
		return nil, ErrCacheExpired
	}

	return &entry, nil
}

// Save atomically writes a cache entry to disk, creating the cache directory if needed.
func (s *FileCacheStore) Save(entry *CacheEntry) error {
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache entry: %w", err)
	}

	// Atomic write: temp file in cache dir + rename (POSIX-safe).
	tmp, err := os.CreateTemp(s.dir, "auth_*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp cache file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("close cache file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path(entry.ProfileName)); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("rename cache file: %w", err)
	}
	return nil
}

// Invalidate removes the cache entry for the given profile name.
func (s *FileCacheStore) Invalidate(profileName string) error {
	err := os.Remove(s.path(profileName))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("remove cache entry %q: %w", profileName, err)
}

func (s *FileCacheStore) path(name string) string {
	return filepath.Join(s.dir, "auth_"+name+".json")
}

// NopCacheStore is a no-op CacheStore used when caching is disabled.
type NopCacheStore struct{}

// Load always returns ErrCacheMiss.
func (NopCacheStore) Load(string) (*CacheEntry, error) { return nil, ErrCacheMiss }

// Save is a no-op.
func (NopCacheStore) Save(*CacheEntry) error { return nil }

// Invalidate is a no-op.
func (NopCacheStore) Invalidate(string) error { return nil }

// Obfuscate encodes value using a profile-name-derived key (XOR + base64).
// This prevents casual reading of cache files but is not encryption.
func Obfuscate(value, profileName string) string {
	key := obfuscationKey(profileName)
	b := []byte(value)
	for i := range b {
		b[i] ^= key[i%len(key)]
	}
	return base64.StdEncoding.EncodeToString(b)
}

// Deobfuscate reverses Obfuscate. Returns ErrCacheCorrupted on invalid input.
func Deobfuscate(encoded, profileName string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: base64 decode: %w", ErrCacheCorrupted, err)
	}
	key := obfuscationKey(profileName)
	for i := range b {
		b[i] ^= key[i%len(key)]
	}
	return string(b), nil
}

func obfuscationKey(profileName string) []byte {
	h := sha256.Sum256([]byte(profileName))
	return h[:]
}
