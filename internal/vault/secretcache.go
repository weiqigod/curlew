package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// secretCacheService is the OS keychain service name for cached secret values.
const secretCacheService = "curlew-secret-cache"

// SecretStore persists fetched secret values across runs. Implementations are
// best-effort: a failed Load is a miss and a failed Save is ignored, so the
// cache can never fail a run.
type SecretStore interface {
	Load(key string) (string, bool)
	Save(key, value string, ttl time.Duration)
}

// SecretCacheKey returns the cache key for path under cfg. The key covers the
// provider and its location settings, so the same path in another vault,
// region or project never shares an entry, and it does not reveal the path.
func SecretCacheKey(cfg *SecretsConfig, path string) string {
	parts := []string{cfg.Provider, cfg.Region, cfg.VaultName, cfg.Address, cfg.Project, path}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// KeyringStore keeps cached secrets in the OS keychain: Windows Credential
// Manager, macOS Keychain or the Linux Secret Service.
type KeyringStore struct {
	Now func() time.Time // nil = time.Now
}

type keyringEntry struct {
	ExpiresAt time.Time `json:"expires_at"`
	Value     string    `json:"value"`
}

func (s *KeyringStore) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Load returns the cached value for key when present and not expired.
// Expired entries are removed.
func (s *KeyringStore) Load(key string) (string, bool) {
	raw, err := keyring.Get(secretCacheService, key)
	if err != nil {
		return "", false
	}
	var entry keyringEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil || !s.now().Before(entry.ExpiresAt) {
		_ = keyring.Delete(secretCacheService, key)
		return "", false
	}
	return entry.Value, true
}

// Save stores value for ttl. Values too large for the keychain are not cached.
func (s *KeyringStore) Save(key, value string, ttl time.Duration) {
	data, err := json.Marshal(keyringEntry{ExpiresAt: s.now().Add(ttl), Value: value})
	if err != nil {
		return
	}
	_ = keyring.Set(secretCacheService, key, string(data))
}

// ClearSecretCache removes every cached secret value from the OS keychain.
func ClearSecretCache() error {
	if err := keyring.DeleteAll(secretCacheService); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("clear secret cache: %w", err)
	}
	return nil
}
