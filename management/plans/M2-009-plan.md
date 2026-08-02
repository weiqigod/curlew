# Implementation Plan: M2-009

## Overview
Extends auth profile execution with file-based token caching (persisting across runs in `.curlew/cache/`) and optional refresh-on-failure behavior that re-executes the auth profile and retries the request when a 401 Unauthorized response is received.

## Task Details
- **ID:** M2-009
- **Title:** Auth profile token caching and refresh
- **Phase:** M2: Dynamic Auth
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-008 | Auth profile configuration and execution | done |

---

## Implementation Steps

### Step 1: Add `CacheTTL` and `RefreshOnFailure` fields to `auth.Profile` and config parsing
**Rationale:** Pure data structure change with no behavioral impact. Zero values preserve all existing behavior (no caching, no refresh). Smallest blast radius — changes no logic.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/profile.go` | modify | Add `CacheTTL int` and `RefreshOnFailure bool` to `Profile` struct |
| `internal/config/project.go` | modify | Add `CacheTTL` and `RefreshOnFailure` to `authProfileEntry`; populate during parsing |
| `internal/config/project_test.go` | modify | Add test cases for new fields |

#### Current Code

```go
// internal/auth/profile.go
type Profile struct {
    Name       string
    Type       ProfileType
    Collection string
    Extract    string
}
```

```go
// internal/config/project.go
type authProfileEntry struct {
    Type       string `yaml:"type"`
    Collection string `yaml:"collection"`
    Extract    string `yaml:"extract,omitempty"`
}
```

#### New Code

```go
// internal/auth/profile.go
type Profile struct {
    Name             string
    Type             ProfileType
    Collection       string
    Extract          string
    CacheTTL         int  // cache_ttl in seconds; 0 = no caching (default)
    RefreshOnFailure bool // retry with fresh auth on 401 responses
}
```

```go
// internal/config/project.go
type authProfileEntry struct {
    Type             string `yaml:"type"`
    Collection       string `yaml:"collection"`
    Extract          string `yaml:"extract,omitempty"`
    CacheTTL         int    `yaml:"cache_ttl,omitempty"`
    RefreshOnFailure bool   `yaml:"refresh_on_failure,omitempty"`
}
```

In the profile construction loop, add the new fields:
```go
cfg.AuthProfiles = append(cfg.AuthProfiles, auth.Profile{
    Name:             name,
    Type:             auth.ProfileDynamic,
    Collection:       entry.Collection,
    Extract:          entry.Extract,
    CacheTTL:         entry.CacheTTL,
    RefreshOnFailure: entry.RefreshOnFailure,
})
```

#### Tests to Write FIRST (RED phase)

```go
// internal/config/project_test.go — add to TestParseProjectConfig_AuthProfiles
func TestParseProjectConfig_AuthProfiles_CacheFields(t *testing.T) {
    tests := []struct {
        name             string
        yaml             string
        wantCacheTTL     int
        wantRefreshOnFail bool
    }{
        {
            "dynamic profile with cache_ttl",
            `auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml\n    cache_ttl: 3600`,
            3600, false,
        },
        {
            "dynamic profile with refresh_on_failure",
            `auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml\n    refresh_on_failure: true`,
            0, true,
        },
        {
            "profile without cache fields defaults to zero",
            `auth_profiles:\n  tok:\n    type: dynamic\n    collection: auth.yaml`,
            0, false,
        },
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected — zero values are the existing defaults.

---

### Step 2: Create `internal/auth/cache.go` — `CacheStore` interface, file implementation, and obfuscation utilities
**Rationale:** Builds the persistence layer in isolation. No callers yet. Can be fully unit-tested before any integration. Defines sentinel errors that later steps reference.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/cache.go` | create | `CacheEntry`, `CacheStore` interface, `FileCacheStore`, `NopCacheStore`, `Obfuscate`/`Deobfuscate` |
| `internal/auth/cache_test.go` | create | Full unit tests for cache and obfuscation |

#### New Code

```go
// internal/auth/cache.go
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

var (
    ErrCacheExpired   = errors.New("auth cache entry expired")
    ErrCacheMiss      = errors.New("auth cache entry not found")
    ErrCacheCorrupted = errors.New("auth cache entry corrupted")
)

// CacheEntry represents cached auth profile credentials.
type CacheEntry struct {
    ProfileName string            `json:"profile_name"`
    Variables   map[string]string `json:"variables"`  // obfuscated values
    ExpiresAt   time.Time         `json:"expires_at"`
    CreatedAt   time.Time         `json:"created_at"`
    Collection  string            `json:"collection"` // detect config drift
}

// CacheStore defines the interface for auth profile credential caching.
type CacheStore interface {
    // Load retrieves a cached entry. Returns ErrCacheMiss, ErrCacheExpired,
    // or ErrCacheCorrupted on failure (all treated as cache miss by callers).
    Load(profileName string) (*CacheEntry, error)
    // Save persists a cache entry. Values are obfuscated before writing.
    Save(entry *CacheEntry) error
    // Invalidate removes the entry. Returns nil if the entry does not exist.
    Invalidate(profileName string) error
}

// FileCacheStore is a CacheStore backed by JSON files in .curlew/cache/.
type FileCacheStore struct {
    dir string
}

// NewFileCacheStore creates a FileCacheStore rooted at projectRoot/.curlew/cache/.
func NewFileCacheStore(projectRoot string) *FileCacheStore {
    return &FileCacheStore{dir: filepath.Join(projectRoot, ".curlew", "cache")}
}

func (s *FileCacheStore) Load(profileName string) (*CacheEntry, error) { ... }
func (s *FileCacheStore) Save(entry *CacheEntry) error { ... }
func (s *FileCacheStore) Invalidate(profileName string) error { ... }
func (s *FileCacheStore) path(profileName string) string {
    return filepath.Join(s.dir, "auth_"+profileName+".json")
}

// NopCacheStore is a no-op CacheStore for when caching is disabled.
type NopCacheStore struct{}
func (NopCacheStore) Load(string) (*CacheEntry, error) { return nil, ErrCacheMiss }
func (NopCacheStore) Save(*CacheEntry) error           { return nil }
func (NopCacheStore) Invalidate(string) error          { return nil }

// Obfuscate encodes value for storage using a profile-specific key.
// This is obfuscation, not encryption — prevents casual reading only.
func Obfuscate(value, profileName string) string { ... }

// Deobfuscate reverses Obfuscate. Returns ErrCacheCorrupted on invalid input.
func Deobfuscate(encoded, profileName string) (string, error) { ... }

func obfuscationKey(profileName string) []byte {
    h := sha256.Sum256([]byte(profileName))
    return h[:]
}
```

**Key implementation details:**
- `Save` uses `os.CreateTemp` in the cache directory followed by `os.Rename` for atomic writes (POSIX-safe for concurrent processes)
- `Load` checks `ExpiresAt` and removes + returns `ErrCacheExpired` if past; removes + returns `ErrCacheCorrupted` if JSON parse fails
- `Obfuscate` applies XOR with the 32-byte SHA-256 key (repeating/wrapping) then base64-encodes
- Cache files stored as `auth_<profile_name>.json`

#### Tests to Write FIRST (RED phase)

```go
// internal/auth/cache_test.go

func TestObfuscate(t *testing.T) {
    tests := []struct {
        name        string
        value       string
        profileName string
    }{
        {"roundtrip simple value", "tok123", "myprofile"},
        {"roundtrip empty value", "", "myprofile"},
        {"roundtrip special chars", "abc!@#$%^&*()\nfoo", "myprofile"},
        {"different values produce different output", "secretA", "myprofile"},
        {"different profiles produce different output", "secret", "profile1"},
    }
    // Roundtrip: Deobfuscate(Obfuscate(v)) == v
    // Distinct: Obfuscate(v, p) != v
    // Profile separation: Obfuscate(v, p1) != Obfuscate(v, p2)
}

func TestDeobfuscateErrors(t *testing.T) {
    tests := []struct {
        name    string
        encoded string
    }{
        {"invalid base64", "not-valid-base64!!!"},
    }
    // errors.Is(err, ErrCacheCorrupted)
}

func TestFileCacheStore(t *testing.T) {
    tests := []struct {
        name        string
        setup       func(t *testing.T, s *FileCacheStore)
        action      func(t *testing.T, s *FileCacheStore) error
        wantErrIs   error
        wantSuccess bool
    }{
        {"save and load roundtrip", /* save then load */, nil, true},
        {"load missing file returns ErrCacheMiss", /* load only */, ErrCacheMiss, false},
        {"load expired entry returns ErrCacheExpired", /* save past expiry */, ErrCacheExpired, false},
        {"load corrupted file returns ErrCacheCorrupted", /* write garbage */, ErrCacheCorrupted, false},
        {"invalidate removes file", /* save then invalidate then load */, ErrCacheMiss, false},
        {"invalidate nonexistent is no-op", /* invalidate only */, nil, true},
        {"save creates cache directory", /* save to new dir */, nil, true},
        {"collection mismatch treated as cache miss", /* save with col A, compare col B */, ErrCacheMiss, false},
    }
}

func TestNopCacheStore(t *testing.T) {
    // Load returns ErrCacheMiss, Save returns nil, Invalidate returns nil
}
```

#### Impact on Existing Tests
- None (new files only).

---

### Step 3: Modify `auth.ExecuteProfiles` to integrate cache lookup and save
**Rationale:** Core behavioral change. Adds cache-hit short-circuit before profile execution, and cache-save after successful execution. Uses zero-value defaults so all existing behavior is preserved.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/profile.go` | modify | Add `CacheStore` parameter to `ExecuteProfiles`; add cache check/save logic in the loop |
| `internal/auth/profile_test.go` | modify | Update all `ExecuteProfiles` calls with `nil` as last arg; add caching test cases |

#### Current Code

```go
// internal/auth/profile.go
func ExecuteProfiles(ctx context.Context, profiles []Profile, projectRoot string, execute ExecuteFunc) (*ProfileResult, error) {
```

#### New Code

```go
// internal/auth/profile.go
func ExecuteProfiles(ctx context.Context, profiles []Profile, projectRoot string, execute ExecuteFunc, cache CacheStore) (*ProfileResult, error) {
    if cache == nil {
        cache = NopCacheStore{}
    }
    result := &ProfileResult{
        Variables: make(map[string]string),
        Sensitive: variable.NewSensitiveSet(),
    }
    for _, p := range profiles {
        // Cache check
        if p.CacheTTL > 0 {
            if entry, err := cache.Load(p.Name); err == nil && entry.Collection == p.Collection {
                // Cache hit: deobfuscate and apply to result
                for k, v := range entry.Variables {
                    plain, deErr := Deobfuscate(v, p.Name)
                    if deErr != nil {
                        _ = cache.Invalidate(p.Name)
                        break // fall through to execute
                    }
                    result.Variables[k] = plain
                    result.Sensitive.Add(plain)
                }
                continue
            }
        }

        // Execute profile (existing logic unchanged)
        vars, err := execute(ctx, collPath)
        if err != nil {
            return nil, fmt.Errorf("auth profile %q: %w: %w", p.Name, ErrProfileFailed, err)
        }
        // Apply Extract filter ... (existing logic)

        // Cache save (if TTL > 0)
        if p.CacheTTL > 0 {
            obfVars := make(map[string]string, len(extracted))
            for k, v := range extracted {
                obfVars[k] = Obfuscate(v, p.Name)
            }
            entry := &CacheEntry{
                ProfileName: p.Name,
                Variables:   obfVars,
                ExpiresAt:   time.Now().Add(time.Duration(p.CacheTTL) * time.Second),
                CreatedAt:   time.Now(),
                Collection:  p.Collection,
            }
            _ = cache.Save(entry) // best-effort; never fail the run
        }
    }
    return result, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/auth/profile_test.go — new test function
func TestExecuteProfiles_Caching(t *testing.T) {
    tests := []struct {
        name          string
        cacheTTL      int
        preloadCache  bool
        wantExecCalls int
    }{
        {"cache hit skips execution", 3600, true, 0},
        {"cache miss executes profile", 3600, false, 1},
        {"zero cache_ttl always executes", 0, true, 1},
        {"nil cache store executes normally", 3600, false /* nil store */, 1},
        {"expired cache re-executes", 3600, false /* expired */, 1},
        {"cached variables marked sensitive", 3600, true, 0},
        {"collection path mismatch ignores cache", 3600, true /* diff collection */, 1},
    }
}

func TestExecuteProfiles_CacheSaveFailure(t *testing.T) {
    // cache.Save returns error → run still succeeds
}
```

#### Impact on Existing Tests
- `internal/auth/profile_test.go` — all existing `ExecuteProfiles(ctx, ...)` calls need `nil` appended as last argument (compile error, trivial fix).
- `internal/runner/runner.go` — the call to `auth.ExecuteProfiles` needs the cache parameter added (see Step 4).

---

### Step 4: Wire `CacheStore` creation in `runner.Run()` and pass to `ExecuteProfiles`
**Rationale:** Connects the cache store to the execution pipeline. Adding `CacheStore` to `VarSources` uses nil as the zero value (no behavior change for existing tests).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `CacheStore` field to `VarSources`; create store in `Run`; pass to `ExecuteProfiles` |
| `internal/runner/runner_test.go` | modify | Add test for second run using cached credentials |

#### Current Code

```go
// internal/runner/runner.go — VarSources struct (approximate)
type VarSources struct {
    // ... existing fields ...
    AuthProfiles    []auth.Profile
    AuthExecuteFunc auth.ExecuteFunc
    ProjectRoot     string
    // ...
}
```

#### New Code

```go
type VarSources struct {
    // ... existing fields unchanged ...
    CacheStore auth.CacheStore // nil = auto-detect from ProjectRoot; non-nil for test injection
}

// In runner.Run(), before calling auth.ExecuteProfiles:
var cacheStore auth.CacheStore
switch {
case vars.CacheStore != nil:
    cacheStore = vars.CacheStore
case vars.ProjectRoot != "":
    cacheStore = auth.NewFileCacheStore(vars.ProjectRoot)
default:
    cacheStore = auth.NopCacheStore{}
}

profileResult, authErr := auth.ExecuteProfiles(ctx, vars.AuthProfiles, vars.ProjectRoot, authExec, cacheStore)
```

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go
func TestRun_AuthProfileCaching(t *testing.T) {
    tests := []struct {
        name          string
        wantExecCalls int
    }{
        {"first run populates cache and calls execute", 1},
        {"second run within TTL hits cache and skips execute", 0},
    }
    // Use a real FileCacheStore in a t.TempDir() as projectRoot
    // Run twice; count how many times authExec is called
}
```

#### Impact on Existing Tests
- `VarSources{}` zero value still works — `CacheStore: nil` triggers auto-detect which falls back to `NopCacheStore{}` when `ProjectRoot` is empty.

---

### Step 5: Implement refresh-on-failure (401 retry) in `executePhase`
**Rationale:** Most complex behavioral change. Isolated to the 401 path — no other behavior is affected. Retry happens at most once per request to prevent loops.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | 401 detection after `exec`; invalidate cache; re-execute auth profile; retry request once |
| `internal/runner/runner_test.go` | modify | Tests for refresh-on-failure scenarios |

#### New Code (outline)

```go
// Inside executePhase, after result := exec(ctx, &req):

// Refresh-on-failure: if 401, auth profile is attached, and refresh is configured
if result != nil && result.StatusCode == 401 && item.Auth != "" && !retried {
    if profile := findAuthProfile(item.Auth, vars.AuthProfiles); profile != nil && profile.RefreshOnFailure {
        // Invalidate stale cache entry
        if cacheStore != nil {
            _ = cacheStore.Invalidate(profile.Name)
        }
        // Re-execute auth profile to get fresh credentials
        freshVars, refreshErr := auth.ExecuteProfiles(
            ctx,
            []auth.Profile{*profile},
            vars.ProjectRoot,
            authExec,
            cacheStore,
        )
        if refreshErr == nil {
            // Update scope with fresh credentials
            for k, v := range freshVars.Variables {
                scope.Set(k, v)
            }
            // Re-interpolate request with fresh scope and retry once
            retried = true
            req = interpolateRequest(item, scope)
            result, execErr = exec(ctx, &req)
            *counter++
        }
        // If refresh fails, fall through to evaluate original 401 result
    }
}
```

Add `findAuthProfile` helper:
```go
func findAuthProfile(name string, profiles []auth.Profile) *auth.Profile {
    for i := range profiles {
        if profiles[i].Name == name {
            return &profiles[i]
        }
    }
    return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecutePhase_RefreshOnFailure(t *testing.T) {
    tests := []struct {
        name              string
        firstStatus       int
        retryStatus       int
        refreshOnFailure  bool
        wantFinalStatus   int
        wantExecCallCount int
    }{
        {"401 with refresh_on_failure retries after re-auth", 401, 200, true, 200, 2},
        {"401 without refresh_on_failure not retried", 401, 200, false, 401, 1},
        {"401 retry only happens once", 401, 401, true, 401, 2},
        {"401 refresh failure falls through to 401", 401, 0 /* auth fails */, true, 401, 1},
        {"403 not retried", 403, 200, true, 403, 1},
        {"401 request without auth field not retried", 401, 200, true /* no item.Auth */, 401, 1},
        {"retry increments request counter by 2", 401, 200, true, 200, 2},
    }
}
```

#### Impact on Existing Tests
- No existing tests exercise the 401 + `RefreshOnFailure` path, so no breakage.
- The `findAuthProfile` helper is package-private — no external impact.

---

### Step 6: Add `.curlew/cache/` to `.gitignore`
**Rationale:** Cache files contain credentials (even if obfuscated) and must not be committed. Low risk, additive only.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `.gitignore` | modify | Add `.curlew/cache/` entry |

#### New Code

```gitignore
# Auth profile credential cache (contains obfuscated sensitive values)
.curlew/cache/
```

#### Impact on Existing Tests
- None.

---

### Step 7: Update smoke test for auth profile caching (if applicable)
**Rationale:** Validates the observable specification end-to-end. The smoke test confirms auth profile executes once when run twice within TTL.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add caching scenario if Solo-tier gating allows it in smoke test context |

Note: Since auth profiles are gated at Solo tier, the smoke test may only validate the feature-gate behavior (exit code 6 for free tier). Full caching smoke test requires a tier override or a test-only bypass.

#### Impact on Existing Tests
- Additive only.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/profile_test.go` | `TestExecuteProfiles` (all cases) | compile error | Add `nil` as last argument to `ExecuteProfiles` calls |
| `internal/runner/runner.go` | `auth.ExecuteProfiles` call | compile error | Pass constructed `cacheStore` variable |
| All other tests | — | none | No changes needed |

---

## Risks and Edge Cases

- **Concurrent writes:** Two processes writing the same cache file → **Mitigation:** Atomic writes using `os.CreateTemp` + `os.Rename` (POSIX atomic).
- **Corrupted cache file:** Invalid JSON or tampered obfuscated values → **Mitigation:** `Load` returns `ErrCacheCorrupted`, caller treats as miss, deletes file, re-executes.
- **Multi-variable profiles (no `extract`):** All variables must be cached post-filter → **Mitigation:** Cache the variables *after* the Extract filter is applied — the exact map that gets set on scope.
- **Retry loop:** `refresh_on_failure` could retry forever → **Mitigation:** `retried` boolean flag ensures at-most-one retry per request.
- **Vault interaction:** Vault secrets used inside auth profile collections resolve fresh each time the auth collection executes → **Mitigation:** Auth cache and vault cache are completely separate systems.
- **Obfuscation security:** XOR+base64 is not encryption → **Mitigation:** Honestly named `Obfuscate`/`Deobfuscate`, `CacheStore` interface allows future AES-256-GCM upgrade for Team/Enterprise tiers.
- **Empty `projectRoot`:** No stable location for cache files → **Mitigation:** `NopCacheStore` used when `projectRoot` is empty.
- **Profile config drift:** User changes `collection` path but cache returns old credentials → **Mitigation:** `CacheEntry.Collection` field checked on `Load`; mismatch treated as miss.

---

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Configure an auth profile with TTL-based caching.
# Run curlew run tests.yaml twice within TTL and confirm auth profile only executes once.
curlew run tests.yaml  # first run: auth profile executes
curlew run tests.yaml  # second run within TTL: auth profile skipped (cached)

# Verify caching and refresh behavior with unit tests:
go test ./internal/auth/...
```
