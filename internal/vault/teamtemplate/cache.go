package teamtemplate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// CacheTTL is the default freshness window. 5 minutes per SPECIFICATION.md:5696.
const CacheTTL = 5 * time.Minute

// FetchTimeout caps a foreground stale-while-revalidate fetch.
// Per task: "≤2s blocking".
const FetchTimeout = 2 * time.Second

// cacheFileName is the on-disk envelope per SPECIFICATION.md:5695.
const cacheFileName = "team_vault.json"

// lockFileName re-uses the License-JWT refresh lock per SPECIFICATION.md:5697.
const lockFileName = "refresh.lock"

// lockTimeout is the maximum time to wait for the flock before giving up.
const lockTimeout = 5 * time.Second

// CacheEnvelope mirrors the JSON shape stored on disk per SPECIFICATION.md:5695.
type CacheEnvelope struct {
	FetchedAt int64  `json:"fetched_at"`
	Version   int64  `json:"version"`
	Template  string `json:"template"` // raw YAML — parsed by teamtemplate.Parse downstream
}

// Fetcher is the minimal contract the cache requires of its backend client.
// Decouples this package from internal/backend to keep tests hermetic.
type Fetcher interface {
	GetTeamVault(ctx context.Context, orgId, accessToken string) (*FetchResult, error)
}

// FetchResult is the cache-layer view of a transport response.
type FetchResult struct {
	Template string
	Version  int64
}

// Cache wraps the on-disk team_vault.json envelope and the refresh lock.
type Cache struct {
	cfgDir string
	now    func() time.Time
}

// NewCache constructs a Cache rooted at cfgDir (typically ~/.config/apitesttool).
func NewCache(cfgDir string) *Cache {
	return &Cache{cfgDir: cfgDir, now: time.Now}
}

// SetCacheClock injects a clock function for tests.
func SetCacheClock(c *Cache, now func() time.Time) {
	c.now = now
}

// Read returns the on-disk envelope, or (nil, os.ErrNotExist) when absent.
func (c *Cache) Read() (*CacheEnvelope, error) {
	data, err := os.ReadFile(filepath.Join(c.cfgDir, cacheFileName))
	if err != nil {
		return nil, err
	}
	var env CacheEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode team_vault.json: %w", err)
	}
	return &env, nil
}

// Write persists env to disk with mode 0600. Atomic via os.Rename of a temp file.
func (c *Cache) Write(env *CacheEnvelope) error {
	if err := os.MkdirAll(c.cfgDir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("encode team_vault.json: %w", err)
	}
	// Write to a temp file then rename for atomicity.
	tmp := filepath.Join(c.cfgDir, cacheFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write team_vault.json.tmp: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(c.cfgDir, cacheFileName)); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename team_vault.json: %w", err)
	}
	return nil
}

// Load returns a fresh-enough template, fetching when necessary per task
// stale-while-revalidate semantics:
//   - no cache + force=true OR no cache          => fetch under flock; persist; return
//   - cache age < TTL AND !force                 => return cached; no network
//   - cache stale OR force                       => fetch (≤2s, under flock); persist; return new
//   - cache stale, fetcher unreachable           => return cached with warning sentinel
//   - no cache + fetcher unreachable             => return nil + the underlying error
//
// warnW receives a one-line stderr message when stale-cache is served (nil = io.Discard).
func (c *Cache) Load(ctx context.Context, fetcher Fetcher, orgId, accessToken string, force bool, warnW io.Writer) (*CacheEnvelope, error) {
	if warnW == nil {
		warnW = io.Discard
	}

	existing, readErr := c.Read()
	cacheExists := readErr == nil && existing != nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		// Corrupt cache: treat as missing.
		cacheExists = false
		existing = nil
	}

	// TTL hit: cache is fresh and no force-refresh.
	if cacheExists && !force {
		age := c.now().Unix() - existing.FetchedAt
		if age >= 0 && time.Duration(age)*time.Second < CacheTTL {
			return existing, nil
		}
	}

	// Cache-only mode is valid for offline workers and for sessions whose
	// token does not carry an organization coordinate. Never dereference a nil
	// fetcher or issue a malformed backend request; stale cached coordinates
	// remain usable until authenticated refresh inputs become available.
	if fetcher == nil || orgId == "" || accessToken == "" {
		if cacheExists {
			return existing, nil
		}
		return nil, nil
	}

	// Need to fetch. Apply the 2s foreground timeout.
	fetchCtx, cancel := context.WithTimeout(ctx, FetchTimeout)
	defer cancel()

	// Acquire the shared lock to prevent thundering-herd.
	release, lockErr := c.lock(ctx)
	if lockErr == nil {
		defer release()
	}
	// If we couldn't acquire the lock, proceed anyway — a sibling may have
	// refreshed the cache while we were waiting. Re-read after timeout.
	if lockErr != nil {
		// Re-read: sibling may have refreshed.
		if fresh, err := c.Read(); err == nil {
			return fresh, nil
		}
	}

	// After acquiring the lock, re-read: a sibling may have written the cache
	// while we were waiting for the lock. Check freshness and bail out early if
	// the sibling's write is sufficient (and we're not doing a forced refresh).
	if lockErr == nil && !force {
		if fresh, err := c.Read(); err == nil {
			freshAge := c.now().Unix() - fresh.FetchedAt
			if freshAge >= 0 && time.Duration(freshAge)*time.Second < CacheTTL {
				return fresh, nil
			}
		}
	}

	result, fetchErr := fetcher.GetTeamVault(fetchCtx, orgId, accessToken)
	if fetchErr != nil {
		if cacheExists {
			_, _ = fmt.Fprintf(warnW, "team vault unavailable, using cached template (age=%s): %v\n",
				time.Duration(c.now().Unix()-existing.FetchedAt)*time.Second, fetchErr)
			return existing, nil
		}
		return nil, fetchErr
	}

	env := &CacheEnvelope{
		FetchedAt: c.now().Unix(),
		Version:   result.Version,
		Template:  result.Template,
	}
	if err := c.Write(env); err != nil {
		// Write failure is non-fatal: return the result anyway.
		_, _ = fmt.Fprintf(warnW, "team vault cache write failed: %v\n", err)
	}
	return env, nil
}

// Refresh always fetches (TTL-bypass) and persists. Used by --refresh-vault and
// by apitest license --refresh after a successful /auth/refresh round-trip.
// Errors are non-fatal for the caller — they may proceed with whatever cache
// exists; warnW receives the error message.
func (c *Cache) Refresh(ctx context.Context, fetcher Fetcher, orgId, accessToken string, warnW io.Writer) error {
	if warnW == nil {
		warnW = io.Discard
	}

	release, lockErr := c.lock(ctx)
	if lockErr == nil {
		defer release()
	}

	result, err := fetcher.GetTeamVault(ctx, orgId, accessToken)
	if err != nil {
		_, _ = fmt.Fprintf(warnW, "team vault refresh failed: %v\n", err)
		return err
	}

	env := &CacheEnvelope{
		FetchedAt: c.now().Unix(),
		Version:   result.Version,
		Template:  result.Template,
	}
	return c.Write(env)
}

// lock acquires the shared refresh.lock with a bounded timeout.
// Returns a release function and nil on success.
func (c *Cache) lock(ctx context.Context) (release func(), err error) {
	lockPath := filepath.Join(c.cfgDir, lockFileName)
	if mkErr := os.MkdirAll(c.cfgDir, 0o700); mkErr != nil {
		return nil, fmt.Errorf("mkdir for lock: %w", mkErr)
	}
	lk := flock.New(lockPath)

	timeout := lockTimeout
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining < timeout {
			timeout = remaining
		}
	}
	lockCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	locked, lockErr := lk.TryLockContext(lockCtx, 25*time.Millisecond)
	if lockErr != nil && !errors.Is(lockErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("flock: %w", lockErr)
	}
	if !locked {
		return nil, fmt.Errorf("flock: timeout acquiring %s", lockPath)
	}
	return func() { _ = lk.Unlock() }, nil
}
