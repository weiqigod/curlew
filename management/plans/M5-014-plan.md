# Implementation Plan: M5-014

## Overview

Deliver the `curlew license export --output <file>` subcommand and the supporting `internal/license/export` package: a gzipped tarball bundle (`license.json`, `jwks.json`, `README.txt`) that lets operators move a validated license onto an air-gapped machine and continue validating offline. Extend `internal/license` so the key resolver and store honor the new `CURLEW_LICENSE_BUNDLE` env var when it points at an extracted bundle, and add a bundle-age grace warning.

## Task Details

- **ID:** M5-014
- **Title:** go-cli: license export bundle for air-gapped use
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** medium
- **Branch:** `feature/M5-014-license-export-bundle`

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-013 | go-cli: offline JWT verification and grace-period state machine | done |

M5-013 shipped the `internal/license` package (store, resolver, validator, JWT parser, embedded JWKS). M5-014 builds on top: the exporter reads the current license.json and JWKS, serializes them into a bundle, and the import path feeds them back into the same `Validator`.

## Architectural Decisions

These resolve open questions before implementation — the executor should follow them unless a hard constraint forces a deviation.

1. **Package layout.** New package `internal/license/export`. Exports three symbols: `Bundle` (struct describing the artifacts), `Writer.Write(ctx, dst) error` to produce a tarball, `Reader.Read(src) (*Bundle, error)` to open an extracted directory. The gzip-tar I/O is a small file `archive.go`; high-level orchestration lives in `export.go`. No sub-sub-package needed.
2. **No external dependencies.** Uses only `archive/tar`, `compress/gzip`, `io`, `os`, `path/filepath`, `time`. Matches `docs/TECH_CHOICES.md` ("standard library first"). No tarball library.
3. **Bundle format.** A single gzipped tarball. Members (top-level, no directory prefix):
   - `license.json` — byte-for-byte copy of `~/.config/curlew/license.json`.
   - `jwks.json` — the JWKS used to verify the cached token. For M5-014 we export the embedded JWKS (`keys.EmbeddedJWKS`), not `jwks_cache.json`. Rationale: the kid used by the token must resolve on import; the embedded JWKS is authoritative and always contains the kid the token was signed with in the current shipping build. A follow-up can add the cached JWKS if kid rotation becomes a concern.
   - `README.txt` — short text explaining: what the bundle is, which files it contains, the single-line env command to activate it on the target machine (`export CURLEW_LICENSE_BUNDLE=./extracted && curlew license --validate`), and the 30-day bundle grace window.
4. **Bundle-age grace.** Bundle age is derived from `license.json`'s `last_validated_at`, not from the tarball mtime — users routinely re-download/copy files. Within 30 days: validate normally. After 30 days (`LastValidatedAt < now - 30d`): print stderr warning `"Bundle grace period expires in N days"` until day 60, then still fall through to the standard GRACE_EXPIRED path from M5-013 (exit 0 for `license --validate`, exit 9 for premium commands). The task YAML's behavior 3 says "Bundle is older than 30 days… still validates if within window" — that window aligns with the existing 30-day grace from M5-013, so implement: if `CURLEW_LICENSE_BUNDLE` is set and `elapsed > 30d`, the validator prints the bundle-specific warning *in addition to* the normal grace warning. The executor must not extend the state machine to 60 days; it reuses the existing `GraceWindow`.
5. **Import wiring.** `CURLEW_LICENSE_BUNDLE=/path/to/extracted` makes the license system read from the bundle instead of `~/.config/curlew/`. Implementation: new helper `license.ResolveConfigDir() string` in `internal/license/paths.go` that returns `CURLEW_LICENSE_BUNDLE` if set (and the directory contains `license.json`), otherwise falls back to `CURLEW_CONFIG_DIR` / `os.UserConfigDir()`. The existing `licenseConfigDir()` in `cmd/curlew/license.go` moves its logic into this helper so the bundle env var is respected by every license-aware command. The bundle path is *read-only*: `Save`/`MarkValidated` from a bundle-backed store must refuse to write (new `ErrReadOnlyBundle`). This is critical — mutating a bundle directory could corrupt an operator's reference copy.
6. **Bundle JWKS loading.** The `KeyResolver` gains a second optional source: if a `jwks.json` exists in the bundle dir, parse it and consult it *before* the regular cached JWKS. The new precedence is: embedded → bundle → cached → online. The bundle JWKS lives in the resolver via a new `bundlePath string` field, nil by default. `NewKeyResolver` gains an optional variadic functional option `WithBundleJWKS(path string)` to keep the existing signature backward-compatible; `NewValidator` wires it when `CURLEW_LICENSE_BUNDLE` is set.
7. **Exit codes.**
   - `0` — successful export (bundle written).
   - `2` — no license on disk / output parent directory missing.
   - `6` — tampered token on import (`signature_invalid`) — existing behavior via Validator.
   - `1` — unexpected internal errors (disk full, permission denied, gzip failure).
8. **Tar tamper detection.** Behavior 6 ("signature tampered after export, imported and validated, exit 6 signature_invalid") is already covered by M5-013: the Validator re-runs `VerifyRS256` on every validate call. The exporter does not need an extra tamper check — tampering with `license.json` inside the extracted directory will fail signature verification on the next `license --validate`. This is the correct layering.
9. **Output size format.** The observable requires `"Wrote bundle.tar.gz (2.1 KB)"`. Implement a tiny `humanBytes(n int64) string` helper in `cmd/curlew/license.go`. Units: B, KB, MB (binary 1024 base, rounded to 1 decimal). No third-party formatter.
10. **JWKS key-count reporting.** The observable requires `"Included: license token (valid until 2027-04-18), JWKS (2 keys)"`. The "valid until" date is taken from the JWT `exp` claim, formatted as `YYYY-MM-DD`. The "(N keys)" count is the length of the JWKS `keys` array. Both are derived at export time, printed to stdout.
11. **Help text and command dispatch.** `licenseCmd` grows a new subcommand branch: `case "export":`. Dispatch to a new `licenseExport(args []string) int` that parses `--output <file>`. `printLicenseHelp()` gets three extra lines documenting `export`, `--output`, and `CURLEW_LICENSE_BUNDLE`. The top-level `printHelp()` comment for the `license` command already says "Manage license state" — no change needed there.
12. **Smoke test.** A new section at the end of `smoke/run.sh`: export to a temp file, untar it, re-validate with `CURLEW_LICENSE_BUNDLE` pointed at the extracted dir, assert `State: VALID`. This is the end-to-end proof demanded by the task's Definition of Done.
13. **Sentinel errors.** Export package exports: `ErrNoLicenseToExport`, `ErrOutputParentMissing`, `ErrBundleIncomplete` (missing required member on import), `ErrReadOnlyBundle` (attempt to Save/MarkValidated from a bundle-backed store).
14. **Context.** `Writer.Write(ctx context.Context, dst io.Writer) error` accepts `context.Context` per project standard, though for file I/O we only check cancellation between tar members.
15. **Do not change the `LicenseStore` interface.** Instead, introduce `license.ReadOnlyStore` that wraps the underlying `*Store` and returns `ErrReadOnlyBundle` on `Save`/`MarkValidated`. `NewValidator` selects which implementation based on whether `CURLEW_LICENSE_BUNDLE` is set. This preserves all existing callers of `LicenseStore`.

## Implementation Steps

Step ordering is smallest blast radius first — pure functions with no callers, then internal plumbing, then the CLI wiring that makes behavior observable.

---

### Step 1: tar/gzip bundle writer and reader (pure, no license deps)

**Rationale:** No other code depends on this. It's a thin wrapper over stdlib; we can land it with full TDD coverage and zero risk to existing tests before any license plumbing changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/license/export/archive.go` | create | `writeTar(w io.Writer, members []tarMember) error` and `readTar(r io.Reader) (map[string][]byte, error)` |
| `internal/license/export/archive_test.go` | create | Round-trip table-driven tests |

#### New Code

```go
// internal/license/export/archive.go
package export

import (
    "archive/tar"
    "compress/gzip"
    "fmt"
    "io"
    "time"
)

// tarMember is a single entry in the bundle.
type tarMember struct {
    Name    string
    Data    []byte
    Mode    int64
    ModTime time.Time
}

// writeTarGz writes members as a gzipped tarball to w.
func writeTarGz(w io.Writer, members []tarMember) error {
    gz := gzip.NewWriter(w)
    defer gz.Close() //nolint:errcheck // error surfaced via gz.Close() below
    tw := tar.NewWriter(gz)
    for _, m := range members {
        hdr := &tar.Header{
            Name:    m.Name,
            Mode:    m.Mode,
            Size:    int64(len(m.Data)),
            ModTime: m.ModTime,
            Typeflag: tar.TypeReg,
        }
        if err := tw.WriteHeader(hdr); err != nil {
            return fmt.Errorf("write header %s: %w", m.Name, err)
        }
        if _, err := tw.Write(m.Data); err != nil {
            return fmt.Errorf("write body %s: %w", m.Name, err)
        }
    }
    if err := tw.Close(); err != nil {
        return fmt.Errorf("close tar: %w", err)
    }
    return gz.Close()
}

// readTarGz reads all members from a gzipped tarball and returns them by name.
func readTarGz(r io.Reader) (map[string][]byte, error) {
    gz, err := gzip.NewReader(r)
    if err != nil {
        return nil, fmt.Errorf("open gzip: %w", err)
    }
    defer gz.Close() //nolint:errcheck
    tr := tar.NewReader(gz)
    out := make(map[string][]byte)
    for {
        hdr, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, fmt.Errorf("read header: %w", err)
        }
        if hdr.Typeflag != tar.TypeReg {
            continue
        }
        data, err := io.ReadAll(tr)
        if err != nil {
            return nil, fmt.Errorf("read body %s: %w", hdr.Name, err)
        }
        out[hdr.Name] = data
    }
    return out, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestTarGzRoundTrip(t *testing.T) {
    tests := []struct {
        name    string
        members []tarMember
    }{
        {"three members", []tarMember{
            {Name: "a.json", Data: []byte(`{"k":1}`), Mode: 0o600, ModTime: time.Unix(100, 0)},
            {Name: "b.json", Data: []byte(`{"k":2}`), Mode: 0o600, ModTime: time.Unix(200, 0)},
            {Name: "README.txt", Data: []byte("hello"), Mode: 0o644, ModTime: time.Unix(300, 0)},
        }},
        {"empty member", []tarMember{
            {Name: "empty.json", Data: []byte{}, Mode: 0o600, ModTime: time.Unix(0, 0)},
        }},
        {"no members", []tarMember{}},
    }
    // build in-memory, read back, verify content by name
}

func TestReadTarGz_InvalidGzip(t *testing.T) {
    _, err := readTarGz(bytes.NewReader([]byte("not gzip")))
    if err == nil { t.Fatal("expected error") }
}
```

#### Impact on Existing Tests
- None. New package.

---

### Step 2: Bundle type with serialize/parse

**Rationale:** A pure in-memory representation of the bundle contents. No file I/O yet. Downstream steps (exporter, importer) compose around this type.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/license/export/bundle.go` | create | `Bundle` struct, `SerializeMembers() []tarMember`, `ParseMembers(map[string][]byte) (*Bundle, error)`, sentinel errors |
| `internal/license/export/bundle_test.go` | create | Round-trip + malformed inputs |

#### New Code

```go
// internal/license/export/bundle.go
package export

import (
    "errors"
    "fmt"
    "time"
)

// Sentinel errors for bundle operations.
var (
    ErrBundleIncomplete    = errors.New("license/export: bundle missing required member")
    ErrNoLicenseToExport   = errors.New("license/export: no license on disk")
    ErrOutputParentMissing = errors.New("license/export: output directory does not exist")
    ErrReadOnlyBundle      = errors.New("license/export: bundle is read-only")
)

// Bundle is the contents of an exported license bundle.
type Bundle struct {
    LicenseJSON []byte // raw license.json bytes
    JWKSJSON    []byte // raw jwks.json bytes
    Readme      []byte // README.txt bytes
    Created     time.Time
}

// required file names inside the tarball.
const (
    FileLicense = "license.json"
    FileJWKS    = "jwks.json"
    FileReadme  = "README.txt"
)

// SerializeMembers returns the tar members in a deterministic order.
func (b *Bundle) SerializeMembers() []tarMember {
    mt := b.Created
    if mt.IsZero() { mt = time.Now() }
    return []tarMember{
        {Name: FileLicense, Data: b.LicenseJSON, Mode: 0o600, ModTime: mt},
        {Name: FileJWKS,    Data: b.JWKSJSON,    Mode: 0o600, ModTime: mt},
        {Name: FileReadme,  Data: b.Readme,      Mode: 0o644, ModTime: mt},
    }
}

// ParseMembers constructs a Bundle from a name→bytes map. Returns
// ErrBundleIncomplete when license.json or jwks.json is missing. README.txt
// is optional on import (tolerate older bundles without one).
func ParseMembers(m map[string][]byte) (*Bundle, error) {
    b := &Bundle{}
    var ok bool
    if b.LicenseJSON, ok = m[FileLicense]; !ok {
        return nil, fmt.Errorf("%w: %s", ErrBundleIncomplete, FileLicense)
    }
    if b.JWKSJSON, ok = m[FileJWKS]; !ok {
        return nil, fmt.Errorf("%w: %s", ErrBundleIncomplete, FileJWKS)
    }
    b.Readme = m[FileReadme]
    return b, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestBundleParseMembers(t *testing.T) {
    tests := []struct {
        name    string
        in      map[string][]byte
        wantErr error
    }{
        {"complete", map[string][]byte{
            FileLicense: []byte(`{"access_token":"x"}`),
            FileJWKS:    []byte(`{"keys":[]}`),
            FileReadme:  []byte("readme"),
        }, nil},
        {"missing license.json", map[string][]byte{
            FileJWKS: []byte(`{"keys":[]}`),
        }, ErrBundleIncomplete},
        {"missing jwks.json", map[string][]byte{
            FileLicense: []byte(`{"access_token":"x"}`),
        }, ErrBundleIncomplete},
        {"readme optional", map[string][]byte{
            FileLicense: []byte(`{"access_token":"x"}`),
            FileJWKS:    []byte(`{"keys":[]}`),
        }, nil},
    }
    // iterate, assert errors.Is
}
```

#### Impact on Existing Tests
- None.

---

### Step 3: Exporter (gather inputs, write tarball)

**Rationale:** Now we plug the stdlib-only tar writer into real license data. Still no CLI wiring — pure library layer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/license/export/export.go` | create | `Write(ctx, cfgDir string, w io.Writer, now func() time.Time) (*Report, error)` |
| `internal/license/export/export_test.go` | create | Happy path + no-license + parent-missing + context-cancel |

#### New Code

```go
// internal/license/export/export.go
package export

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "time"

    "github.com/weiqigod/curlew/internal/license"
    "github.com/weiqigod/curlew/internal/license/jwks"
    "github.com/weiqigod/curlew/internal/license/keys"
)

// Report is the post-export summary printed by the CLI.
type Report struct {
    TokenExpiresAt time.Time // JWT exp claim
    JWKSKeyCount   int       // keys in the embedded JWKS
    BytesWritten   int64
}

// Write gathers license.json and the embedded JWKS from cfgDir/binary and
// writes a gzipped bundle to w. Returns ErrNoLicenseToExport when no
// license.json is present. Returns ctx.Err() if ctx is canceled.
func Write(ctx context.Context, cfgDir string, w io.Writer, now func() time.Time) (*Report, error) {
    if err := ctx.Err(); err != nil { return nil, err }

    lj, err := os.ReadFile(filepath.Join(cfgDir, "license.json"))
    if errors.Is(err, os.ErrNotExist) {
        return nil, ErrNoLicenseToExport
    }
    if err != nil {
        return nil, fmt.Errorf("read license.json: %w", err)
    }

    // Parse the token once, for the expiry report and validation.
    var rec license.LicenseRecord
    if err := json.Unmarshal(lj, &rec); err != nil {
        return nil, fmt.Errorf("parse license.json: %w", err)
    }
    tok, err := license.ParseToken(rec.AccessToken)
    if err != nil {
        return nil, fmt.Errorf("parse token: %w", err)
    }

    set, err := jwks.ParseJWKS(keys.EmbeddedJWKS)
    if err != nil {
        return nil, fmt.Errorf("parse embedded JWKS: %w", err)
    }

    readme := renderReadme(now())
    b := &Bundle{
        LicenseJSON: lj,
        JWKSJSON:    keys.EmbeddedJWKS,
        Readme:      []byte(readme),
        Created:     now(),
    }

    cw := &countingWriter{w: w}
    if err := writeTarGz(cw, b.SerializeMembers()); err != nil {
        return nil, err
    }
    return &Report{
        TokenExpiresAt: time.Unix(tok.Claims.Exp, 0).UTC(),
        JWKSKeyCount:   len(set.Keys),
        BytesWritten:   cw.n,
    }, nil
}

// renderReadme returns the README.txt text for a bundle created at now.
func renderReadme(now time.Time) string { /* ~15 lines of fixed prose */ }

type countingWriter struct{ w io.Writer; n int64 }
func (c *countingWriter) Write(p []byte) (int, error) {
    n, err := c.w.Write(p); c.n += int64(n); return n, err
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWrite_Happy(t *testing.T) {
    dir := t.TempDir()
    // write a fixture license.json using testdata/private.pem to mint a token.
    // call Write to a bytes.Buffer, then readTarGz and assert three members present.
    // assert Report.TokenExpiresAt matches minted exp; JWKSKeyCount == 1; BytesWritten > 0.
}

func TestWrite_NoLicense(t *testing.T) {
    dir := t.TempDir()
    _, err := Write(context.Background(), dir, io.Discard, time.Now)
    if !errors.Is(err, ErrNoLicenseToExport) { t.Fatalf("want ErrNoLicenseToExport, got %v", err) }
}

func TestWrite_ContextCanceled(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background()); cancel()
    _, err := Write(ctx, t.TempDir(), io.Discard, time.Now)
    if !errors.Is(err, context.Canceled) { t.Fatalf("want context.Canceled, got %v", err) }
}
```

#### Impact on Existing Tests
- None. Pure new code.

---

### Step 4: Bundle-aware config-dir resolver and read-only store

**Rationale:** Small targeted change to `internal/license`. We introduce the env var plumbing here so the CLI can both export *from* the real config dir and import *into* a bundle path. This ships before the CLI wiring because the CLI step will rely on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/license/paths.go` | create | `ResolveConfigDir(lookup func(string) string) (dir string, fromBundle bool, err error)` |
| `internal/license/paths_test.go` | create | Table-driven tests for env var precedence |
| `internal/license/store.go` | modify | Add `ReadOnlyStore` wrapper returning `ErrReadOnlyBundle` from Save/MarkValidated |
| `internal/license/store_test.go` | modify | New tests for `ReadOnlyStore` |
| `internal/license/resolver.go` | modify | Add `bundlePath` field + `WithBundleJWKS(path string)` functional option; insert bundle lookup step 2 in `Resolve` |
| `internal/license/resolver_test.go` | modify | New test `TestKeyResolver_BundleJWKS` |
| `internal/license/validator.go` | modify | `NewValidator` uses `ResolveConfigDir`; wires `ReadOnlyStore` and `WithBundleJWKS` when `fromBundle==true` |

#### Current Code

```go
// cmd/curlew/license.go — moves into internal/license/paths.go
func licenseConfigDir() (string, error) {
    if override := os.Getenv("CURLEW_CONFIG_DIR"); override != "" {
        return override, nil
    }
    base, err := os.UserConfigDir()
    if err != nil {
        return "", fmt.Errorf("locate config dir: %w", err)
    }
    return filepath.Join(base, "curlew"), nil
}
```

```go
// internal/license/resolver.go — current signature
func NewKeyResolver(cfgDir string, online OnlineFetcher, isOffline func() bool) (*KeyResolver, error)
```

#### New Code

```go
// internal/license/paths.go
package license

import (
    "fmt"
    "os"
    "path/filepath"
)

// BundleEnv is the env var pointing at an extracted license bundle directory.
const BundleEnv = "CURLEW_LICENSE_BUNDLE"

// ConfigEnv overrides the default ~/.config/curlew directory.
const ConfigEnv = "CURLEW_CONFIG_DIR"

// ResolveConfigDir returns the directory to use for license state files and
// whether it is a read-only bundle directory. Precedence:
//   1. CURLEW_LICENSE_BUNDLE (fromBundle=true, read-only)
//   2. CURLEW_CONFIG_DIR
//   3. os.UserConfigDir()/curlew
func ResolveConfigDir() (dir string, fromBundle bool, err error) {
    if b := os.Getenv(BundleEnv); b != "" {
        return b, true, nil
    }
    if c := os.Getenv(ConfigEnv); c != "" {
        return c, false, nil
    }
    base, err := os.UserConfigDir()
    if err != nil {
        return "", false, fmt.Errorf("locate config dir: %w", err)
    }
    return filepath.Join(base, "curlew"), false, nil
}
```

```go
// internal/license/store.go — addition
// ReadOnlyStore wraps a LicenseStore to refuse mutating operations. Used when
// reading from an CURLEW_LICENSE_BUNDLE directory.
type ReadOnlyStore struct{ Inner LicenseStore }

func (r *ReadOnlyStore) Load() (*LicenseRecord, error) { return r.Inner.Load() }
func (r *ReadOnlyStore) Save(*LicenseRecord) error     { return ErrReadOnlyBundle }
func (r *ReadOnlyStore) MarkValidated(*LicenseRecord, time.Time) error { return ErrReadOnlyBundle }

// ErrReadOnlyBundle is returned when mutating an CURLEW_LICENSE_BUNDLE-backed store.
var ErrReadOnlyBundle = errors.New("license: bundle is read-only")
```

```go
// internal/license/resolver.go — modification
type KeyResolver struct {
    embedded   *jwks.Set
    bundlePath string // optional: path to bundle jwks.json
    cachePath  string
    online     OnlineFetcher
    isOffline  func() bool
}

// Option configures NewKeyResolver.
type Option func(*KeyResolver)

// WithBundleJWKS makes the resolver consult an extracted bundle's jwks.json
// after the embedded JWKS but before the cache.
func WithBundleJWKS(path string) Option {
    return func(r *KeyResolver) { r.bundlePath = path }
}

func NewKeyResolver(cfgDir string, online OnlineFetcher, isOffline func() bool, opts ...Option) (*KeyResolver, error) {
    // existing body … then apply opts.
}

// Resolve lookup order: embedded → bundle → cached → online.
func (r *KeyResolver) Resolve(kid string) (*rsa.PublicKey, KeySource, error) {
    if pk, ok := r.embedded.LookupRSA(kid); ok { return pk, SourceEmbedded, nil }
    if r.bundlePath != "" {
        if data, err := os.ReadFile(r.bundlePath); err == nil {
            if set, err := jwks.ParseJWKS(data); err == nil {
                if pk, ok := set.LookupRSA(kid); ok { return pk, SourceBundle, nil }
            }
        }
    }
    // existing cached / online / error branches unchanged
}

// New KeySource constant:
const SourceBundle KeySource = "bundle JWKS"
```

```go
// internal/license/validator.go — modification
func NewValidator(cfgDir string) (*Validator, error) { // keep existing signature for back-compat
    _, fromBundle, _ := ResolveConfigDir()
    var opts []Option
    if fromBundle { opts = append(opts, WithBundleJWKS(filepath.Join(cfgDir, "jwks.json"))) }
    res, err := NewKeyResolver(cfgDir, nil, OfflineEnv, opts...)
    if err != nil { return nil, fmt.Errorf("build key resolver: %w", err) }
    var store LicenseStore = NewStore(cfgDir)
    if fromBundle { store = &ReadOnlyStore{Inner: store} }
    return &Validator{Store: store, Resolver: res, Now: time.Now}, nil
}
```

And `cmd/curlew/license.go`'s `licenseConfigDir()` becomes a one-liner:

```go
func licenseConfigDir() (string, error) {
    dir, _, err := license.ResolveConfigDir()
    return dir, err
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/license/paths_test.go
func TestResolveConfigDir(t *testing.T) {
    tests := []struct{
        name                 string
        bundle, cfg          string
        wantSuffix           string
        wantFromBundle       bool
    }{
        {"bundle wins", "/b", "/c", "/b", true},
        {"config when no bundle", "", "/c", "/c", false},
        {"fallback to userconfigdir", "", "", "curlew", false},
    }
    // t.Setenv for each; assert.
}

// internal/license/store_test.go
func TestReadOnlyStore_RefusesWrites(t *testing.T) {
    ro := &ReadOnlyStore{Inner: NewStore(t.TempDir())}
    if err := ro.Save(&LicenseRecord{}); !errors.Is(err, ErrReadOnlyBundle) { t.Fatal(err) }
    if err := ro.MarkValidated(&LicenseRecord{}, time.Now()); !errors.Is(err, ErrReadOnlyBundle) { t.Fatal(err) }
}

// internal/license/resolver_test.go
func TestKeyResolver_BundleJWKS(t *testing.T) {
    bundleDir := t.TempDir()
    cfgDir := t.TempDir()
    // copy jwks_extra.json (kid=curlew-2025-02) into bundleDir/jwks.json
    res, _ := NewKeyResolver(cfgDir, nil, func() bool { return true },
        WithBundleJWKS(filepath.Join(bundleDir, "jwks.json")))
    _, src, err := res.Resolve("curlew-2025-02")
    if err != nil { t.Fatal(err) }
    if src != SourceBundle { t.Errorf("src=%s want bundle", src) }
}
```

#### Impact on Existing Tests

| Test | Impact | Action |
|------|--------|--------|
| `TestKeyResolver_Embedded` | None — signature back-compat via variadic opts. | No change. |
| `TestKeyResolver_CachedFallback` | None — still no bundle, falls through to cache. | No change. |
| `TestKeyResolver_OfflineFails` | None. | No change. |
| `TestKeyResolver_OnlineFetcherCachesResult` | None. | No change. |
| `TestKeyResolver_SaveCached` | None. | No change. |
| All tests in `cmd/curlew/license_test.go` that use `CURLEW_CONFIG_DIR` | None — `ResolveConfigDir` preserves existing precedence. Add a clear-env: `t.Setenv("CURLEW_LICENSE_BUNDLE", "")` in licenseTestSetup helper to avoid cross-test pollution. | Audit + add a safety line (see Test Impact Summary). |

---

### Step 5: `curlew license export --output` CLI wiring

**Rationale:** With the library in place, this is pure string-parsing + orchestration. One observable is unlocked by this step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/license.go` | modify | New `licenseExport(args []string) int`; register under `case "export":`; update `printLicenseHelp()` |
| `cmd/curlew/license_test.go` | modify | Add export happy-path + error-path tests using `captureRun` |

#### Current Code

```go
switch args[0] {
case "--validate":
    return licenseValidate()
case "--refresh", "--debug":
    _, _ = fmt.Fprintf(os.Stderr, "Not yet implemented: %s\n", args[0])
    return 1
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown license flag: %s\n", args[0])
    printLicenseHelp()
    return 1
}
```

#### New Code

```go
switch args[0] {
case "--validate":
    return licenseValidate()
case "export":
    return licenseExport(args[1:])
case "--refresh", "--debug":
    _, _ = fmt.Fprintf(os.Stderr, "Not yet implemented: %s\n", args[0])
    return 1
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown license flag: %s\n", args[0])
    printLicenseHelp()
    return 1
}
```

```go
// licenseExport implements `curlew license export --output <file>`.
// Exit codes: 0 success; 2 no license / parent dir missing; 1 I/O error.
func licenseExport(args []string) int {
    var output string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--output", "-o":
            i++
            if i >= len(args) { _, _ = fmt.Fprintln(os.Stderr, "error: --output requires a file path"); return 2 }
            output = args[i]
        case "--help", "-h":
            printLicenseHelp(); return 0
        default:
            _, _ = fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i]); return 2
        }
    }
    if output == "" { _, _ = fmt.Fprintln(os.Stderr, "error: --output is required"); return 2 }

    parent := filepath.Dir(output)
    if parent != "" && parent != "." {
        if _, err := os.Stat(parent); os.IsNotExist(err) {
            _, _ = fmt.Fprintln(os.Stderr, "error: output directory does not exist"); return 2
        }
    }

    cfgDir, err := licenseConfigDir()
    if err != nil { _, _ = fmt.Fprintln(os.Stderr, err); return 1 }

    fmt.Println("Exporting license bundle...")

    f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
    if err != nil { _, _ = fmt.Fprintf(os.Stderr, "error: open output: %v\n", err); return 1 }
    defer f.Close()

    rep, err := export.Write(context.Background(), cfgDir, f, time.Now)
    if err != nil {
        if errors.Is(err, export.ErrNoLicenseToExport) {
            _, _ = fmt.Fprintln(os.Stderr, "error: no license to export; run `curlew login` first")
            return 2
        }
        _, _ = fmt.Fprintf(os.Stderr, "error: export failed: %v\n", err)
        return 1
    }
    fmt.Printf("Included: license token (valid until %s), JWKS (%d keys)\n",
        rep.TokenExpiresAt.Format("2006-01-02"), rep.JWKSKeyCount)
    fmt.Printf("Wrote %s (%s)\n", output, humanBytes(rep.BytesWritten))
    return 0
}

// humanBytes renders n as B/KB/MB using 1024-base, 1 decimal for KB/MB.
func humanBytes(n int64) string {
    const k = 1024.0
    switch {
    case n < 1024: return fmt.Sprintf("%d B", n)
    case n < 1024*1024: return fmt.Sprintf("%.1f KB", float64(n)/k)
    default: return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
    }
}
```

And `printLicenseHelp()` gains:

```
  export --output <file>   Export the cached license and embedded JWKS as a
                           gzipped tarball (license.json, jwks.json, README.txt)
                           for air-gapped use. On the target machine set
                           CURLEW_LICENSE_BUNDLE=<extracted-dir> to validate.
```

Plus an env-var block addition:

```
  CURLEW_LICENSE_BUNDLE=dir   Read license+JWKS from an extracted bundle (read-only)
```

#### Tests to Write FIRST (RED phase)

```go
func TestLicenseExport_HappyPath(t *testing.T) {
    // setup cfg dir with valid license.json fixture
    out := filepath.Join(t.TempDir(), "bundle.tar.gz")
    t.Setenv("CURLEW_CONFIG_DIR", cfgDir)
    t.Setenv("CURLEW_LICENSE_BUNDLE", "")
    stdout, _, rc := captureRun(t, "license", "export", "--output", out)
    if rc != 0 { t.Fatalf("rc=%d", rc) }
    if !strings.Contains(stdout, "Exporting license bundle...") { t.Error("missing header") }
    if !strings.Contains(stdout, "Included: license token (valid until") { t.Error("missing report") }
    if !strings.Contains(stdout, "JWKS (1 keys)") { t.Error("missing jwks count") }
    // verify file exists, is gzip, contains license.json/jwks.json/README.txt
}

func TestLicenseExport_NoLicense(t *testing.T) {
    t.Setenv("CURLEW_CONFIG_DIR", t.TempDir())
    _, stderr, rc := captureRun(t, "license", "export", "--output", filepath.Join(t.TempDir(), "x.tgz"))
    if rc != 2 { t.Fatalf("rc=%d", rc) }
    if !strings.Contains(stderr, "no license to export") { t.Error("missing message") }
    if !strings.Contains(stderr, "curlew login") { t.Error("missing hint") }
}

func TestLicenseExport_OutputParentMissing(t *testing.T) {
    cfg := setupValidLicense(t)
    t.Setenv("CURLEW_CONFIG_DIR", cfg)
    _, stderr, rc := captureRun(t, "license", "export", "--output", "/nonexistent/dir/x.tgz")
    if rc != 2 { t.Fatalf("rc=%d", rc) }
    if !strings.Contains(stderr, "output directory does not exist") { t.Error(stderr) }
}

func TestLicenseExport_MissingOutputFlag(t *testing.T) {
    _, stderr, rc := captureRun(t, "license", "export")
    if rc != 2 { t.Fatalf("rc=%d", rc) }
    if !strings.Contains(stderr, "--output is required") { t.Error(stderr) }
}

func TestLicenseHelp_DocumentsExport(t *testing.T) {
    stdout, _, rc := captureRun(t, "license", "--help")
    if rc != 0 || !strings.Contains(stdout, "export --output") { t.Error("missing export doc") }
    if !strings.Contains(stdout, "CURLEW_LICENSE_BUNDLE") { t.Error("missing env var doc") }
}
```

#### Impact on Existing Tests
- `TestLicenseHelp` already asserts `--validate` and `CURLEW_OFFLINE`. It keeps passing because we only *add* lines to help text. New test `TestLicenseHelp_DocumentsExport` covers the new content.
- All `cmd/curlew/license_test.go` tests must ensure `CURLEW_LICENSE_BUNDLE` is unset (or test-local) to avoid the bundle path leaking between tests. Add `t.Setenv("CURLEW_LICENSE_BUNDLE", "")` to the shared setup block of existing tests (behavior-preserving).

---

### Step 6: End-to-end import-and-validate test + bundle-age warning

**Rationale:** Behavior 2 ("extract bundle, set env, validate on fresh machine") and behavior 3 ("bundle older than 30 days prints warning") need integration coverage. This is the most load-bearing behavior test in the task.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/license_test.go` | modify | `TestLicenseExportThenValidate`, `TestLicenseBundleGraceWarning` |
| `cmd/curlew/license.go` | modify | In `licenseValidate()`, when `CURLEW_LICENSE_BUNDLE` is set and the elapsed time since `LastValidatedAt` exceeds 30d, also print `"Bundle grace period expires in N days"` before exiting. |
| `internal/license/validator.go` | modify | Add `FromBundle bool` to `Result` so the CLI can branch on it without re-reading env. |

#### Current Code

```go
// Validator.Validate returns *Result without a FromBundle flag.
type Result struct {
    State State
    ...
}
```

#### New Code

```go
type Result struct {
    State     State
    ...
    FromBundle bool // true when loaded via CURLEW_LICENSE_BUNDLE
}

// Validator gains a FromBundle field populated by NewValidator.
type Validator struct {
    Store      LicenseStore
    Resolver   *KeyResolver
    Now        func() time.Time
    FromBundle bool
}
```

`Validate` sets `res.FromBundle = v.FromBundle`.

`cmd/curlew/license.go` addition in `licenseValidate`:

```go
if res.FromBundle && (res.State == license.StateGracePeriod || res.State == license.StateGraceExpired) {
    days := res.Decision.DaysUntilExpiry
    if days < 0 { days = 0 }
    _, _ = fmt.Fprintf(os.Stderr, "Bundle grace period expires in %d days\n", days)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestLicenseExportThenValidate_RoundTrip(t *testing.T) {
    // 1. set up a valid license in cfgA, CURLEW_CONFIG_DIR=cfgA
    // 2. run `license export --output /tmp/X/bundle.tgz`
    // 3. tar -xzf /tmp/X/bundle.tgz -C /tmp/Y   (or use readTarGz to extract)
    // 4. unset CURLEW_CONFIG_DIR; set CURLEW_LICENSE_BUNDLE=/tmp/Y
    // 5. run `license --validate`; expect exit 0, State: VALID, stdout key source "bundle JWKS" OR "embedded JWKS" (embedded wins; acceptable).
    //    Accept either; primary assertion is exit 0 + State: VALID.
}

func TestLicenseBundleGraceWarning(t *testing.T) {
    // same setup as above; additionally CURLEW_LAST_VALIDATION_OVERRIDE=25d
    // assert stderr contains "Bundle grace period expires in 5 days"
    // assert exit 0
}
```

#### Impact on Existing Tests

| Test | Impact | Action |
|------|--------|--------|
| `TestLicenseValidate_Offline_Valid` | none (FromBundle=false by default) | no change |
| `TestLicenseValidate_Offline_GracePeriod_Day25` | none | no change |
| `TestLicenseValidate_Offline_GraceExpired_Day31` | none | no change |
| `TestRunCmd_GraceExpired_ExitsNine` | none (bundle path not set) | no change |

---

### Step 7: Smoke test — export → extract → validate

**Rationale:** Definition of Done explicitly requires a smoke test covering the export → import → validate cycle.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Append a new `=== License Bundle Export (M5-014) ===` section |

#### New Code (shell)

```bash
echo "=== License Bundle Export (M5-014) ==="

BUNDLE_DIR=$(mktemp -d /tmp/curlew_bundle_XXXXXX)
BUNDLE_FILE="$BUNDLE_DIR/bundle.tar.gz"

# Export using the same fixture license dir as the offline tests.
export CURLEW_CONFIG_DIR="$LICENSE_CFG"
unset CURLEW_LICENSE_BUNDLE
OUT=$(./curlew license export --output "$BUNDLE_FILE" 2>&1)
echo "$OUT" | grep -q "Wrote $BUNDLE_FILE" \
  && echo "PASS: bundle exported" || { echo "FAIL: export — $OUT"; exit 1; }

# Extract and validate offline on a "fresh machine" (empty config dir).
EXTRACT_DIR=$(mktemp -d /tmp/curlew_extract_XXXXXX)
tar -xzf "$BUNDLE_FILE" -C "$EXTRACT_DIR"
[ -f "$EXTRACT_DIR/license.json" ] && [ -f "$EXTRACT_DIR/jwks.json" ] && [ -f "$EXTRACT_DIR/README.txt" ] \
  && echo "PASS: bundle contains all 3 files" || { echo "FAIL: missing bundle members"; exit 1; }

unset CURLEW_CONFIG_DIR
export CURLEW_LICENSE_BUNDLE="$EXTRACT_DIR"
OUT=$(CURLEW_OFFLINE=1 CURLEW_LAST_VALIDATION_OVERRIDE=1h ./curlew license --validate 2>&1)
echo "$OUT" | grep -q "State: VALID" \
  && echo "PASS: bundle validates offline" || { echo "FAIL: bundle validate — $OUT"; exit 1; }

unset CURLEW_LICENSE_BUNDLE
```

#### Impact on Existing Tests
- None. Appends a new section after the M5-013 block; variable names are namespaced (`BUNDLE_*`, `EXTRACT_*`).

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/license/resolver_test.go` | existing 5 tests | none (variadic opts preserve signature) | no change |
| `internal/license/store_test.go` | existing tests | none | add `TestReadOnlyStore_RefusesWrites` (new) |
| `internal/license/validator_test.go` | existing tests | none (new field defaults to false) | audit — likely no change |
| `cmd/curlew/license_test.go` | existing 10 tests | minor — bundle env must be explicitly cleared | prepend `t.Setenv("CURLEW_LICENSE_BUNDLE", "")` in existing tests that set `CURLEW_CONFIG_DIR` |
| `cmd/curlew/license_test.go` | new tests | — | add 7 new tests (Step 5 x5, Step 6 x2) |
| `internal/license/paths_test.go` | new file | — | 3 table cases |
| `internal/license/export/*_test.go` | new files | — | archive_test (3 cases), bundle_test (4 cases), export_test (3 cases) = **10 tests** — hits the ≥6 bar |
| `smoke/run.sh` | new section | none (additive) | ensure `EXTRACT_DIR`/`BUNDLE_*` cleanup on success |

## Risks and Edge Cases

- **Risk:** Changing `NewKeyResolver` signature breaks callers. → **Mitigation:** Add options via variadic, keep the three positional params identical. All existing call sites compile unchanged.
- **Risk:** `CURLEW_LICENSE_BUNDLE` leaks across tests in the same package via `os.Setenv`. → **Mitigation:** Always use `t.Setenv`, and prepend an explicit `t.Setenv("CURLEW_LICENSE_BUNDLE", "")` to existing tests that rely on `CURLEW_CONFIG_DIR`.
- **Risk:** The tarball is not reproducible byte-for-byte (ModTime differs). → **Mitigation:** Bundle reproducibility is not in scope. If we need it later, the `Bundle.Created` field gives us one knob to fix it.
- **Edge case:** `--output` path has no parent (e.g. `bundle.tar.gz` with cwd-relative). → **Handling:** We skip the parent-existence check when `filepath.Dir(output)` is `"."` or empty.
- **Edge case:** The license file on disk is a *bundle* copy (user sets `CURLEW_LICENSE_BUNDLE` then runs `license export`). → **Handling:** Allowed — `Write` reads by path, so re-exporting an imported bundle works. Smoke is still clean because export writes to a *new* file.
- **Edge case:** Empty `--output` path (e.g. `--output ""`). → **Handling:** Treated as missing; exit 2 with the same message as "no output".
- **Edge case:** Output file already exists. → **Handling:** We open with `O_CREATE|O_WRONLY|O_TRUNC`, overwriting silently. This matches Unix conventions; the executor may optionally add an `--overwrite` guard later but the task does not require it.
- **Edge case:** README.txt hard-codes a date that goes stale. → **Mitigation:** Render README at export time using `now()` so its "Generated on YYYY-MM-DD" line is accurate; the rest is static.
- **Edge case:** JWKS from bundle differs from embedded (future key rotation). → **Handling:** Bundle source ranks above cache but below embedded; if embedded already has the kid, we use it. Bundle wins only for kids not in the embedded set.
- **Risk:** `--validate` with `CURLEW_LICENSE_BUNDLE` pointing at a dir missing `license.json`. → **Mitigation:** `Store.Load` returns `ErrNoLicense`, validator emits `State: NO_LICENSE`, CLI prints "no license" and exits 2. Same as today.
- **Edge case:** `CURLEW_LICENSE_BUNDLE` points at a **file** rather than a directory. → **Handling:** `os.ReadFile(filepath.Join(path, "license.json"))` returns an error that is *not* `os.ErrNotExist` (it's `ENOTDIR`). The store surfaces the raw error wrapped as "read license". CLI exits 1 with a clear message.
- **Risk:** Behavior 6 (tamper → exit 6). Already covered transitively — tampering the extracted `license.json` fails `VerifyRS256` which maps to `ErrSignatureInvalid` and exit 6 in `licenseValidate`. We add a dedicated test to confirm this path survives the refactor.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/license/export/...
go test ./internal/license/...
go test ./cmd/curlew/...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# 1. export works
./curlew license export --output bundle.tar.gz
# Expected stdout:
#   Exporting license bundle...
#   Included: license token (valid until YYYY-MM-DD), JWKS (1 keys)
#   Wrote bundle.tar.gz (2.1 KB)     # size varies; unit must be KB

# 2. bundle members present
tar -tzf bundle.tar.gz
# Expected: license.json, jwks.json, README.txt (any order)

# 3. import and validate on a "fresh" dir
mkdir /tmp/extracted && tar -xzf bundle.tar.gz -C /tmp/extracted
CURLEW_LICENSE_BUNDLE=/tmp/extracted CURLEW_OFFLINE=1 ./curlew license --validate
# Expected: exit 0, State: VALID

# 4. help documents the new command + env var
./curlew license --help | grep -E 'export --output|CURLEW_LICENSE_BUNDLE'
```
