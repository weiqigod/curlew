package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestExecuteProfiles(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		profiles    []Profile
		projectRoot string
		execute     ExecuteFunc
		wantVars    map[string]string
		wantErr     error
	}{
		{
			name:     "empty profiles returns empty result",
			profiles: nil,
			execute:  nil, // should not be called
			wantVars: map[string]string{},
		},
		{
			name: "single profile extracts all vars",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml"},
			},
			execute: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{"admin_token": "tok123", "expires": "3600"}, nil
			},
			wantVars: map[string]string{"admin_token": "tok123", "expires": "3600"},
		},
		{
			name: "profile with extract field filters to one variable",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml", Extract: "admin_token"},
			},
			execute: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{"admin_token": "tok123", "other": "ignored"}, nil
			},
			wantVars: map[string]string{"admin_token": "tok123"},
		},
		{
			name: "profile execution failure wraps ErrProfileFailed",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml"},
			},
			execute: func(_ context.Context, _ string) (map[string]string, error) {
				return nil, errors.New("connection refused")
			},
			wantErr: ErrProfileFailed,
		},
		{
			name: "extract variable not in output wraps ErrProfileNoExtract",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml", Extract: "missing_var"},
			},
			execute: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{"other": "val"}, nil
			},
			wantErr: ErrProfileNoExtract,
		},
		{
			name: "projectRoot prepended to relative collection path",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml"},
			},
			projectRoot: "/project",
			execute: func(_ context.Context, path string) (map[string]string, error) {
				if path != "/project/auth/login.yaml" {
					return nil, fmt.Errorf("unexpected path: %s", path)
				}
				return map[string]string{"token": "x"}, nil
			},
			wantVars: map[string]string{"token": "x"},
		},
		{
			name: "all returned variables are marked sensitive",
			profiles: []Profile{
				{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml"},
			},
			execute: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{"token": "x", "secret": "y"}, nil
			},
			wantVars: map[string]string{"token": "x", "secret": "y"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExecuteProfiles(ctx, tt.profiles, tt.projectRoot, tt.execute, nil)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got err %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Variables) != len(tt.wantVars) {
				t.Errorf("variables count = %d, want %d: got %v, want %v",
					len(result.Variables), len(tt.wantVars), result.Variables, tt.wantVars)
			}
			for k, want := range tt.wantVars {
				if got := result.Variables[k]; got != want {
					t.Errorf("Variables[%q] = %q, want %q", k, got, want)
				}
			}
			// Verify all returned variables are marked sensitive
			for k := range result.Variables {
				if !result.Sensitive.IsSensitive(k) {
					t.Errorf("variable %q not marked sensitive", k)
				}
			}
		})
	}
}

// mockCacheStore is an in-memory CacheStore for testing.
type mockCacheStore struct {
	entries    map[string]*CacheEntry
	saveErr    error
	loadErrMap map[string]error // profile-specific load errors
}

func newMockCacheStore() *mockCacheStore {
	return &mockCacheStore{
		entries:    make(map[string]*CacheEntry),
		loadErrMap: make(map[string]error),
	}
}

func (m *mockCacheStore) Load(profileName string) (*CacheEntry, error) {
	if err, ok := m.loadErrMap[profileName]; ok {
		return nil, err
	}
	e, ok := m.entries[profileName]
	if !ok {
		return nil, ErrCacheMiss
	}
	if time.Now().After(e.ExpiresAt) {
		return nil, ErrCacheExpired
	}
	return e, nil
}

func (m *mockCacheStore) Save(entry *CacheEntry) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.entries[entry.ProfileName] = entry
	return nil
}

func (m *mockCacheStore) Invalidate(profileName string) error {
	delete(m.entries, profileName)
	return nil
}

func (m *mockCacheStore) preload(profileName, collection string, vars map[string]string, ttl time.Duration) {
	obf := make(map[string]string, len(vars))
	for k, v := range vars {
		obf[k] = Obfuscate(v, profileName)
	}
	m.entries[profileName] = &CacheEntry{
		ProfileName: profileName,
		Variables:   obf,
		ExpiresAt:   time.Now().Add(ttl),
		CreatedAt:   time.Now(),
		Collection:  collection,
	}
}

func TestExecuteProfiles_Caching(t *testing.T) {
	ctx := context.Background()
	profile := Profile{
		Name:       "login",
		Type:       ProfileDynamic,
		Collection: "auth/login.yaml",
		CacheTTL:   3600,
	}

	tests := []struct {
		name          string
		cacheTTL      int
		setupCache    func(s *mockCacheStore)
		wantExecCalls int
		wantToken     string
	}{
		{
			name:     "cache hit skips execution",
			cacheTTL: 3600,
			setupCache: func(s *mockCacheStore) {
				s.preload("login", "auth/login.yaml", map[string]string{"admin_token": "cached_tok"}, time.Hour)
			},
			wantExecCalls: 0,
			wantToken:     "cached_tok",
		},
		{
			name:          "cache miss executes profile",
			cacheTTL:      3600,
			setupCache:    func(_ *mockCacheStore) {},
			wantExecCalls: 1,
			wantToken:     "fresh_tok",
		},
		{
			name:     "zero cache_ttl always executes",
			cacheTTL: 0,
			setupCache: func(s *mockCacheStore) {
				s.preload("login", "auth/login.yaml", map[string]string{"admin_token": "cached_tok"}, time.Hour)
			},
			wantExecCalls: 1,
			wantToken:     "fresh_tok",
		},
		{
			name:     "expired cache re-executes",
			cacheTTL: 3600,
			setupCache: func(s *mockCacheStore) {
				s.preload("login", "auth/login.yaml", map[string]string{"admin_token": "old_tok"}, -time.Second)
			},
			wantExecCalls: 1,
			wantToken:     "fresh_tok",
		},
		{
			name:     "collection path mismatch ignores cache",
			cacheTTL: 3600,
			setupCache: func(s *mockCacheStore) {
				s.preload("login", "auth/other.yaml", map[string]string{"admin_token": "stale_tok"}, time.Hour)
			},
			wantExecCalls: 1,
			wantToken:     "fresh_tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execCalls := 0
			execute := func(_ context.Context, _ string) (map[string]string, error) {
				execCalls++
				return map[string]string{"admin_token": "fresh_tok"}, nil
			}

			p := profile
			p.CacheTTL = tt.cacheTTL

			store := newMockCacheStore()
			tt.setupCache(store)

			result, err := ExecuteProfiles(ctx, []Profile{p}, "", execute, store)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if execCalls != tt.wantExecCalls {
				t.Errorf("execute calls = %d, want %d", execCalls, tt.wantExecCalls)
			}
			if got := result.Variables["admin_token"]; got != tt.wantToken {
				t.Errorf("admin_token = %q, want %q", got, tt.wantToken)
			}
		})
	}

	t.Run("nil cache store executes normally", func(t *testing.T) {
		execCalls := 0
		execute := func(_ context.Context, _ string) (map[string]string, error) {
			execCalls++
			return map[string]string{"admin_token": "fresh_tok"}, nil
		}
		p := profile
		result, err := ExecuteProfiles(ctx, []Profile{p}, "", execute, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if execCalls != 1 {
			t.Errorf("execute calls = %d, want 1", execCalls)
		}
		if result.Variables["admin_token"] != "fresh_tok" {
			t.Errorf("unexpected token: %q", result.Variables["admin_token"])
		}
	})

	t.Run("cached variables marked sensitive", func(t *testing.T) {
		execute := func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{}, nil
		}
		store := newMockCacheStore()
		store.preload("login", "auth/login.yaml", map[string]string{"admin_token": "cached_tok"}, time.Hour)

		result, err := ExecuteProfiles(ctx, []Profile{profile}, "", execute, store)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Sensitive.IsSensitive("admin_token") {
			t.Error("cached variable admin_token not marked sensitive")
		}
	})
}

func TestExecuteProfiles_CacheSaveFailure(t *testing.T) {
	ctx := context.Background()
	execute := func(_ context.Context, _ string) (map[string]string, error) {
		return map[string]string{"admin_token": "tok"}, nil
	}
	store := newMockCacheStore()
	store.saveErr = errors.New("disk full")

	profile := Profile{Name: "login", Type: ProfileDynamic, Collection: "auth/login.yaml", CacheTTL: 3600}
	result, err := ExecuteProfiles(ctx, []Profile{profile}, "", execute, store)
	if err != nil {
		t.Fatalf("save failure should not fail the run, got: %v", err)
	}
	if result.Variables["admin_token"] != "tok" {
		t.Errorf("unexpected token: %q", result.Variables["admin_token"])
	}
}
