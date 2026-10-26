package vault

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// memoryStore is an in-memory SecretStore that records what was saved.
type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
	ttls   map[string]time.Duration
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: map[string]string{}, ttls: map[string]time.Duration{}}
}

func (m *memoryStore) Load(key string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[key]
	return v, ok
}

func (m *memoryStore) Save(key, value string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	m.ttls[key] = ttl
}

// countingOp answers `op read <path>` commands and records each call.
type countingOp struct {
	mu    sync.Mutex
	calls []string
}

func (c *countingOp) exec(_ context.Context, command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, command)
	return "value-of-" + command[strings.LastIndex(command, "/")+1:], nil
}

func onePasswordConfig(ttl int) *SecretsConfig {
	return &SecretsConfig{
		Provider: Provider1Password,
		Keys: map[string]string{
			"dev_token":  "op://Vault/dev/password",
			"test_token": "op://Vault/test/password",
			"prod_token": "op://Vault/prod/password",
		},
		CacheTTL: ttl,
	}
}

func TestResolveWith_OnlyFetchesSelectedKeys(t *testing.T) {
	op := &countingOp{}
	result, err := ResolveWith(context.Background(), onePasswordConfig(0), op.exec, ResolveOptions{
		Only: map[string]bool{"dev_token": true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.calls) != 1 || !strings.Contains(op.calls[0], "op://Vault/dev/password") {
		t.Fatalf("op calls = %v, want only the dev reference", op.calls)
	}
	if len(result.Variables) != 1 || result.Variables["dev_token"] == "" {
		t.Fatalf("variables = %v, want only dev_token", result.Variables)
	}
}

func TestResolveWith_NilOnlyFetchesAllKeys(t *testing.T) {
	op := &countingOp{}
	result, err := ResolveWith(context.Background(), onePasswordConfig(0), op.exec, ResolveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.calls) != 3 || len(result.Variables) != 3 {
		t.Fatalf("calls=%d variables=%d, want 3 and 3", len(op.calls), len(result.Variables))
	}
}

func TestResolveWith_CacheServesLaterRunsWithoutProvider(t *testing.T) {
	store := newMemoryStore()
	cfg := onePasswordConfig(3600)

	first := &countingOp{}
	r1, err := ResolveWith(context.Background(), cfg, first.exec, ResolveOptions{Store: store})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(first.calls) != 3 {
		t.Fatalf("first run op calls = %d, want 3", len(first.calls))
	}
	for key, ttl := range store.ttls {
		if ttl != time.Hour {
			t.Errorf("saved %s with ttl %v, want 1h", key, ttl)
		}
	}

	second := &countingOp{}
	r2, err := ResolveWith(context.Background(), cfg, second.exec, ResolveOptions{Store: store})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(second.calls) != 0 {
		t.Fatalf("second run op calls = %v, want none", second.calls)
	}
	for name, v := range r1.Variables {
		if r2.Variables[name] != v {
			t.Errorf("%s = %q from cache, want %q", name, r2.Variables[name], v)
		}
	}
}

func TestResolveWith_FetchesOnlyCacheMisses(t *testing.T) {
	store := newMemoryStore()
	cfg := onePasswordConfig(3600)
	store.Save(SecretCacheKey(cfg, "op://Vault/dev/password"), "cached-dev", time.Hour)

	op := &countingOp{}
	result, err := ResolveWith(context.Background(), cfg, op.exec, ResolveOptions{Store: store})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.calls) != 2 {
		t.Fatalf("op calls = %v, want the two misses only", op.calls)
	}
	if result.Variables["dev_token"] != "cached-dev" {
		t.Errorf("dev_token = %q, want cached value", result.Variables["dev_token"])
	}
}

func TestResolveWith_ZeroTTLIgnoresStore(t *testing.T) {
	store := newMemoryStore()
	cfg := onePasswordConfig(0)
	store.Save(SecretCacheKey(cfg, "op://Vault/dev/password"), "stale", time.Hour)

	op := &countingOp{}
	result, err := ResolveWith(context.Background(), cfg, op.exec, ResolveOptions{Store: store, Only: map[string]bool{"dev_token": true}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(op.calls) != 1 || result.Variables["dev_token"] == "stale" {
		t.Fatalf("calls=%v dev_token=%q, want a fresh fetch", op.calls, result.Variables["dev_token"])
	}
	if len(store.values) != 1 {
		t.Errorf("store written with ttl 0: %v", store.values)
	}
}

func TestSecretCacheKey_SeparatesProviderSettings(t *testing.T) {
	a := &SecretsConfig{Provider: ProviderAzure, VaultName: "kv-a"}
	b := &SecretsConfig{Provider: ProviderAzure, VaultName: "kv-b"}
	if SecretCacheKey(a, "secret") == SecretCacheKey(b, "secret") {
		t.Fatal("same path in different vaults must not share a cache entry")
	}
	if SecretCacheKey(a, "secret") != SecretCacheKey(a, "secret") {
		t.Fatal("cache key must be stable")
	}
	if strings.Contains(SecretCacheKey(a, "secret"), "secret") {
		t.Fatal("cache key must not reveal the secret path")
	}
}

func TestKeyringStore_RoundTripExpiryAndClear(t *testing.T) {
	keyring.MockInit()
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	store := &KeyringStore{Now: func() time.Time { return now }}

	store.Save("k1", "v1", time.Hour)
	if v, ok := store.Load("k1"); !ok || v != "v1" {
		t.Fatalf("Load = %q, %v; want v1, true", v, ok)
	}

	now = now.Add(2 * time.Hour)
	if _, ok := store.Load("k1"); ok {
		t.Fatal("expired entry must miss")
	}
	if _, err := keyring.Get(secretCacheService, "k1"); err == nil {
		t.Fatal("expired entry must be removed from the keyring")
	}

	store.Save("k2", "v2", time.Hour)
	if err := ClearSecretCache(); err != nil {
		t.Fatalf("ClearSecretCache: %v", err)
	}
	if _, ok := store.Load("k2"); ok {
		t.Fatal("cleared entry must miss")
	}
}

func TestKeyringStore_UnavailableKeyringIsAMiss(t *testing.T) {
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	store := &KeyringStore{}
	store.Save("k", "v", time.Hour)
	if _, ok := store.Load("k"); ok {
		t.Fatal("an unavailable keyring must behave as a cache miss")
	}
}
