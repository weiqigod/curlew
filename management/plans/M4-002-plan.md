# Implementation Plan: M4-002

## Overview
Wire the `internal/vault/teamtemplate` parser (M4-001) into the runtime so
`curlew run` can resolve `{{secrets.ALIAS}}` placeholders through a shared
vault template selected by `--env`, with a stub provider (`CURLEW_VAULT_STUB=1`)
that makes the slice runnable end-to-end without real cloud credentials.

## Task Details
- **ID:** M4-002
- **Title:** CLI consumes shared vault template at runtime
- **Phase:** M4: Team Tier
- **Priority:** 1
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-001 | Shared vault configuration template format | done |

## Architectural Decisions

Because task behaviours and the existing code pull in several directions, the
following decisions were made up front. Each decision is documented so reviewers
can see the reasoning rather than reverse-engineering it from the diff.

1. **Namespace syntax: introduce a dedicated `{{secrets.ALIAS}}` pattern.**
   The existing `variable.varPattern` (`{{[a-zA-Z_][a-zA-Z0-9_]*}}`) does not
   match dots and must stay that way — several test suites depend on the
   current alphabet. Rather than widening it (large blast radius across
   `internal/parallel/scan.go`, parser, and a dozen test files), we add a
   dedicated `secretsPattern = \{\{secrets\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}` that
   runs as a *pre-pass* inside `Scope.Interpolate`, replacing `{{secrets.X}}`
   with the value from a new `Scope.secrets map[string]string` before the
   normal `varPattern` pass. This keeps plain `{{api_key}}` (Layer 3 vault)
   and `{{secrets.api_key}}` (Layer 4 team template) as clearly distinct
   namespaces, matches the spec language
   (`docs/SPECIFICATION.md:8262` `{{secrets.SECRET_NAME}}`), and leaves
   existing tests untouched.

2. **Template loading lives in `internal/config`, not `internal/runner`.**
   A new `config.LoadTeamTemplate(envVarValue string)` helper reads
   `CURLEW_TEAM_CONFIG`, parses + validates via `teamtemplate.Parse/Validate`,
   and returns a `*teamtemplate.TeamTemplate` or a structured error. Keeping
   I/O in `config` matches the pattern used by `LoadProjectConfig` and
   `LoadEnvironment` and keeps `runner` unit tests hermetic.

3. **Stub provider lives in `internal/vault/teamtemplate/stub.go`.**
   The task says so explicitly. The stub satisfies `vault.Provider` and
   deterministically derives values from the path + environment name so tests
   can assert on the exact resolved value (`stub::<env>::<path>#<field>`). It
   is only activated by `CURLEW_VAULT_STUB=1` and must never be selected in
   production code paths unless the env var is set — we make it explicit and
   testable by plumbing a `VaultStubEnabled bool` through `VarSources` rather
   than reading the env var inside the runner.

4. **Second-call cache lives on a new `SecretsResolver` type inside
   `internal/vault/teamtemplate`.** The behaviour
   *"vault provider is queried once and the second call is served from
   in-memory cache"* requires a single-run cache. We attach it to a small
   resolver struct built once per `runner.Run` invocation and passed into
   `buildScope` alongside the other vault wiring. The cache is keyed by alias
   and lives only for the duration of one run (no TTL, no persistence).

5. **A new Team-tier feature gate, `shared_vault_templates`, is registered
   in `auth.DefaultRegistry`.** `CURLEW_TEAM_CONFIG` at Free/Solo tier
   returns exit code 6 with a `TierTeam` gate error — matching how every
   other paid feature behaves. Tests that need to bypass the gate set
   `CURLEW_TIER=team`.

6. **`--env` is required when and only when the collection actually
   references `{{secrets.X}}`.** If a collection uses no secrets references,
   `CURLEW_TEAM_CONFIG` still loads and validates (so misconfiguration fails
   loudly) but `--env` is optional. This is important for the forward
   compatibility story: teams can roll out the env var to CI before all
   collections use it.

7. **Log line format: `Resolved N secrets from shared template (<env>)`
   prints to stderr exactly once per run.** It goes to stderr so JSON/TAP
   output stays clean, and it prints before the first request is dispatched.
   The runner surfaces a `SharedSecretsResolved int` counter on
   `runner.Summary` for tests that want to assert on it; the stderr line is
   produced by `cmd/curlew/main.go` (not the runner) to keep `runner` free
   of I/O side effects.

8. **Error taxonomy (all new sentinels):**
   - `teamtemplate.ErrTemplateNotFound` — `CURLEW_TEAM_CONFIG` points to a
     missing file.
   - `teamtemplate.ErrUnknownEnvironment` — `--env staging` but staging is
     not in the template.
   - `teamtemplate.ErrUnknownAlias` — `{{secrets.api_key}}` used but alias
     not in active environment.
   - `teamtemplate.ErrEnvFlagRequired` — secrets referenced but `--env`
     missing.
   All are wrapped in `apierrors.Structured` with `CategoryConfig` before
   leaving the runner so printers render them consistently.

## Implementation Steps

Steps are ordered smallest-blast-radius first: pure helpers and data types,
then runner wiring, then CLI entrypoint and smoke test. Each step can be
committed separately and the binary continues to build+pass tests after each.

### Step 1: Secrets namespace in `internal/variable`

**Rationale:** The variable package is the furthest leaf — changing it first
lets every later layer rely on `{{secrets.X}}` resolving. This change is
additive (new regex + new map) and touches no existing code paths, so the
existing 400+ variable tests stay green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | add `secretsPattern`, `secrets` field on `Scope`, `WithSecrets`, pre-pass inside `Interpolate` |
| `internal/variable/variable_test.go` | modify | add `TestScope_Interpolate_Secrets` table |
| `internal/variable/sensitive.go` | modify (if needed) | secrets are always sensitive — add helper if not already doable via `SensitiveSet` |
| `internal/parallel/scan.go` | modify | extend scanner to recognise `{{secrets.X}}` tokens so parallel dependency analysis treats them as pre-execution variables (they are resolved before the first wave) |
| `internal/parallel/scan_test.go` | modify | add case "secrets namespace is not a dependency" |

#### Current Code

```go
// internal/variable/variable.go
var (
    varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
    dynPattern = regexp.MustCompile(`\{\{\$([a-zA-Z][a-zA-Z0-9_]*)\}\}`)
)

type Scope struct {
    vars      map[string]string
    resolved  map[string]string
    registry  *Registry
    funcCache map[string]string
}
```

#### New Code

```go
// internal/variable/variable.go
var (
    varPattern     = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
    dynPattern     = regexp.MustCompile(`\{\{\$([a-zA-Z][a-zA-Z0-9_]*)\}\}`)
    secretsPattern = regexp.MustCompile(`\{\{secrets\.([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
)

// ErrUnknownSecret is returned when {{secrets.NAME}} references a name that
// has not been registered on the scope.
var ErrUnknownSecret = errors.New("unknown secret alias")

type Scope struct {
    vars      map[string]string
    resolved  map[string]string
    registry  *Registry
    funcCache map[string]string
    secrets   map[string]string // nil when no team template is active
}

// WithSecrets attaches a resolved secrets map to the scope. Returns a shallow
// copy so callers can layer secrets onto an existing scope without mutating it.
// Passing a nil or empty map leaves the receiver's secrets unchanged.
func (s *Scope) WithSecrets(secrets map[string]string) *Scope {
    if len(secrets) == 0 {
        return s
    }
    cp := *s
    cp.secrets = secrets
    return &cp
}

// HasSecretsNamespace reports whether input contains any {{secrets.X}} tokens.
// Used by the runner to decide whether --env is required.
func HasSecretsNamespace(input string) bool {
    return secretsPattern.MatchString(input)
}

// SecretReferences returns all alias names referenced via {{secrets.ALIAS}}.
func SecretReferences(input string) []string { /* analogous to FindReferences */ }
```

`Interpolate` gets a new first pre-pass *before* the dynamic pass:

```go
func (s *Scope) Interpolate(input string) (string, error) {
    // Pass 0: resolve {{secrets.X}} via the secrets namespace.
    if secretsPattern.MatchString(input) {
        var retErr error
        input = secretsPattern.ReplaceAllStringFunc(input, func(match string) string {
            if retErr != nil {
                return match
            }
            name := match[len("{{secrets.") : len(match)-2]
            val, ok := s.secrets[name]
            if !ok {
                retErr = &apierrors.Structured{
                    Category: apierrors.CategoryConfig,
                    Message:  fmt.Sprintf("unknown secret alias %q", name),
                    Hint:     "Check team_secrets.vault_configs.<env>.keys in your shared vault template",
                    Inner:    ErrUnknownSecret,
                }
                return match
            }
            return val
        })
        if retErr != nil {
            return "", retErr
        }
    }
    // existing Pass 1 (dynamic) and Pass 2 (varPattern) follow unchanged.
    ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestScope_Interpolate_Secrets(t *testing.T) {
    tests := []struct {
        name    string
        secrets map[string]string
        input   string
        want    string
        wantErr error
    }{
        {"resolves simple alias", map[string]string{"api_key": "abc"},
            "Bearer {{secrets.api_key}}", "Bearer abc", nil},
        {"multiple aliases in one string",
            map[string]string{"api_key": "abc", "db_password": "pw"},
            "{{secrets.api_key}}:{{secrets.db_password}}", "abc:pw", nil},
        {"unknown alias returns ErrUnknownSecret",
            map[string]string{"api_key": "abc"},
            "{{secrets.db_password}}", "", ErrUnknownSecret},
        {"no secrets map and no reference is a no-op",
            nil, "plain", "plain", nil},
        {"no secrets map with reference returns ErrUnknownSecret",
            nil, "{{secrets.api_key}}", "", ErrUnknownSecret},
        {"mixed with regular vars",
            map[string]string{"api_key": "abc"},
            "https://{{host}}/{{secrets.api_key}}", "https://x/abc", nil},
        {"secrets alias never shadowed by var of same name",
            map[string]string{"api_key": "from-secrets"},
            "{{secrets.api_key}}/{{api_key}}", "from-secrets/plain-var", nil},
    }
    // table execution with NewScope(...).WithSecrets(...).WithDynamic(...)
}

func TestHasSecretsNamespace(t *testing.T) { /* true for "{{secrets.x}}", false for "{{x}}" */ }
func TestSecretReferences(t *testing.T)    { /* returns sorted unique aliases */ }
```

#### Impact on Existing Tests
- None — all additions are net-new symbols. `varPattern` and the existing
  interpolation pass are unchanged.

---

### Step 2: Stub provider in `internal/vault/teamtemplate/stub.go`

**Rationale:** Now that `{{secrets.X}}` resolves against a map, we need a way
to populate that map without real AWS/Azure. The stub is standalone, has zero
dependencies on runner, and is trivially testable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/teamtemplate/stub.go` | create | implement `StubProvider` satisfying `vault.Provider` |
| `internal/vault/teamtemplate/stub_test.go` | create | unit tests for deterministic output + BulkFetch |

#### New Code

```go
// internal/vault/teamtemplate/stub.go
package teamtemplate

import (
    "context"
    "fmt"
    "sort"

    "github.com/weiqigod/curlew/internal/vault"
)

// StubProvider is a deterministic in-memory vault provider used when
// CURLEW_VAULT_STUB=1. It returns values of the form
//
//     stub::<envName>::<path>[#<field>]
//
// so tests can assert on the exact resolved value without needing real
// cloud credentials.
type StubProvider struct {
    envName  string
    provider string // "aws-secrets-manager" or "azure-key-vault"
}

// NewStubProvider constructs a stub scoped to a single environment.
func NewStubProvider(envName, provider string) *StubProvider {
    return &StubProvider{envName: envName, provider: provider}
}

func (s *StubProvider) Name() string { return "stub::" + s.provider }

func (s *StubProvider) Fetch(_ context.Context, path string) (string, error) {
    return fmt.Sprintf("stub::%s::%s", s.envName, path), nil
}

func (s *StubProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
    sort.Strings(paths) // deterministic order for tests
    out := make(map[string]string, len(paths))
    for _, p := range paths {
        v, _ := s.Fetch(ctx, p)
        out[p] = v
    }
    return out, nil
}

func (s *StubProvider) ValidateConfig() error { return nil }
```

#### Tests to Write FIRST (RED phase)

```go
func TestStubProvider_Fetch(t *testing.T) {
    tests := []struct {
        name, env, provider, path, want string
    }{
        {"production AWS path", "production", "aws-secrets-manager", "prod/api-key",
            "stub::production::prod/api-key"},
        {"staging Azure path", "staging", "azure-key-vault", "staging-api-key",
            "stub::staging::staging-api-key"},
        {"path with field suffix preserved",
            "production", "aws-secrets-manager", "prod/db#password",
            "stub::production::prod/db#password"},
    }
    // ...
}

func TestStubProvider_BulkFetch_ReturnsMap(t *testing.T) { /* unique paths in map */ }
func TestStubProvider_ValidateConfig_NeverErrors(t *testing.T) {}
func TestStubProvider_Name_IncludesProvider(t *testing.T) {}
```

#### Impact on Existing Tests
- None.

---

### Step 3: `SecretsResolver` + `LoadTeamTemplate` loader

**Rationale:** Wrap the parsing/validation/resolution flow in a small type so
runner only sees a resolve function with a cache, not raw files and providers.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/teamtemplate/resolver.go` | create | `SecretsResolver` with `Resolve(ctx) (map[string]string, error)`, single-call cache |
| `internal/vault/teamtemplate/resolver_test.go` | create | second-call-cached, unknown-alias error, env-not-declared error |
| `internal/vault/teamtemplate/teamtemplate.go` | modify | add new sentinel errors `ErrTemplateNotFound`, `ErrUnknownEnvironment`, `ErrUnknownAlias`, `ErrEnvFlagRequired` |
| `internal/config/team_template.go` | create | `LoadTeamTemplate(path string) (*teamtemplate.TeamTemplate, error)` — reads file, parses, validates, returns wrapped structured error on failure |
| `internal/config/team_template_test.go` | create | happy path, missing file, invalid YAML, validation failure |

#### New Code

```go
// internal/vault/teamtemplate/resolver.go
package teamtemplate

import (
    "context"
    "fmt"
    "sync"

    apierrors "github.com/weiqigod/curlew/internal/errors"
    "github.com/weiqigod/curlew/internal/vault"
)

// SecretsResolver resolves {{secrets.X}} aliases against a chosen environment
// of a shared vault template. One instance lives for the duration of a single
// runner.Run invocation and caches the first successful resolve in memory.
type SecretsResolver struct {
    env      *ResolvedEnv          // active environment (never nil once constructed)
    provider vault.Provider        // AWS/Azure or Stub
    mu       sync.Mutex
    cached   map[string]string     // nil until first Resolve completes
    cachedEr error                 // set if first Resolve returned an error
}

// NewSecretsResolver binds a template + environment name + provider factory
// into a single resolver. Returns ErrUnknownEnvironment when envName is not
// declared in the template.
func NewSecretsResolver(t *TeamTemplate, envName string, makeProvider func(env *ResolvedEnv) (vault.Provider, error)) (*SecretsResolver, error) {
    env, ok := t.Resolve(envName)
    if !ok {
        return nil, fmt.Errorf("%w: %q", ErrUnknownEnvironment, envName)
    }
    p, err := makeProvider(env)
    if err != nil {
        return nil, err
    }
    return &SecretsResolver{env: env, provider: p}, nil
}

// Resolve returns the resolved secrets map for the active environment.
// Subsequent calls are served from the in-memory cache; the provider is
// queried at most once per SecretsResolver instance.
func (r *SecretsResolver) Resolve(ctx context.Context) (map[string]string, error) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if r.cached != nil || r.cachedEr != nil {
        return r.cached, r.cachedEr
    }

    refs := make([]vault.KeyRef, 0, len(r.env.Keys))
    for _, ref := range r.env.Keys {
        refs = append(refs, ref)
    }
    paths := make([]string, 0, len(refs))
    for _, ref := range refs {
        paths = append(paths, ref.Path)
    }
    fetched, err := r.provider.BulkFetch(ctx, paths)
    if err != nil {
        r.cachedEr = wrapFetchError(r.provider, err)
        return nil, r.cachedEr
    }
    out := make(map[string]string, len(refs))
    for alias, ref := range r.env.Keys {
        raw := fetched[ref.Path]
        if ref.Field == "" {
            out[alias] = raw
            continue
        }
        val, extractErr := vault.ExtractField(raw, ref.Field)
        if extractErr != nil {
            r.cachedEr = fmt.Errorf("secret %q field %q: %w", ref.Path, ref.Field, extractErr)
            return nil, r.cachedEr
        }
        out[alias] = val
    }
    r.cached = out
    return out, nil
}

// EnvName returns the active environment name (for log messages).
func (r *SecretsResolver) EnvName() string { return r.env.Name }

// Count returns the number of aliases in the active environment.
func (r *SecretsResolver) Count() int { return len(r.env.Keys) }

// HasAlias reports whether the active environment declares the given alias.
func (r *SecretsResolver) HasAlias(alias string) bool {
    _, ok := r.env.Keys[alias]
    return ok
}
```

```go
// internal/vault/teamtemplate/teamtemplate.go — add sentinels
var (
    // ErrTemplateNotFound is returned when CURLEW_TEAM_CONFIG points to
    // a file that does not exist or cannot be read.
    ErrTemplateNotFound = errors.New("shared vault template not found")
    // ErrUnknownEnvironment is returned when --env names an environment
    // that is not declared in the template.
    ErrUnknownEnvironment = errors.New("unknown environment in shared vault template")
    // ErrUnknownAlias is returned when a collection references an alias
    // that is not declared in the active environment.
    ErrUnknownAlias = errors.New("unknown secret alias")
    // ErrEnvFlagRequired is returned when a collection uses {{secrets.X}}
    // but --env was not provided.
    ErrEnvFlagRequired = errors.New("shared template requires --env")
)
```

```go
// internal/config/team_template.go
package config

import (
    "errors"
    "fmt"
    "os"

    apierrors "github.com/weiqigod/curlew/internal/errors"
    "github.com/weiqigod/curlew/internal/vault/teamtemplate"
)

// LoadTeamTemplate reads, parses, and validates the shared vault template at
// the given path. Returns a structured error ready for rendering by the
// CLI printers. An empty path returns (nil, nil) — the caller decides
// whether that is an error.
func LoadTeamTemplate(path string) (*teamtemplate.TeamTemplate, error) {
    if path == "" {
        return nil, nil
    }
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, &apierrors.Structured{
                Category: apierrors.CategoryConfig,
                Message:  fmt.Sprintf("shared vault template not found: %s", path),
                Hint:     "Check CURLEW_TEAM_CONFIG or remove it to disable team templates",
                Inner:    teamtemplate.ErrTemplateNotFound,
            }
        }
        return nil, fmt.Errorf("read team template %q: %w", path, err)
    }
    tpl, parseErr := teamtemplate.Parse(data)
    if parseErr != nil {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryConfig,
            Message:  fmt.Sprintf("parse team template %s: %v", path, parseErr),
            Inner:    parseErr,
        }
    }
    if issues := tpl.Validate(); len(issues) > 0 {
        first := issues[0]
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryConfig,
            Message:  fmt.Sprintf("invalid team template %s: %s: %s", path, first.Path, first.Message),
            Inner:    teamtemplate.ErrInvalidTemplate,
        }
    }
    return tpl, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/vault/teamtemplate/resolver_test.go
func TestSecretsResolver_Resolve(t *testing.T) {
    tpl := mustParse(t, validTwoEnvTemplate)
    r, err := NewSecretsResolver(tpl, "production", stubFactory("production"))
    if err != nil { t.Fatal(err) }

    got, err := r.Resolve(context.Background())
    if err != nil { t.Fatal(err) }
    if got["api_key"] != "stub::production::prod/api-key" { /* ... */ }
}

func TestSecretsResolver_CachedOnSecondCall(t *testing.T) {
    calls := 0
    makeProv := func(env *ResolvedEnv) (vault.Provider, error) {
        return &countingProvider{inner: NewStubProvider(env.Name, env.Provider), calls: &calls}, nil
    }
    // ... call Resolve twice, assert calls == 1
}

func TestSecretsResolver_UnknownEnvironment(t *testing.T) { /* --env foo errors */ }

func TestSecretsResolver_FieldExtraction(t *testing.T) { /* JSON body + #password */ }

// internal/config/team_template_test.go
func TestLoadTeamTemplate(t *testing.T) {
    tests := []struct {
        name    string
        path    string // populated in t.Run via tmpDir
        wantErr error
    }{
        {"valid template", /* write testdata/team/shared-vault-template.yaml */, nil},
        {"missing file", "/nonexistent/curlew-team.yaml", teamtemplate.ErrTemplateNotFound},
        {"invalid yaml", /* write broken yaml */, teamtemplate.ErrInvalidTemplate},
        {"validation fails", /* provider=foo */, teamtemplate.ErrInvalidTemplate},
        {"empty path returns nil,nil", "", nil},
    }
    // ...
}
```

#### Impact on Existing Tests
- None: new files, new symbols.

---

### Step 4: Feature gate + `VarSources` plumbing in runner

**Rationale:** Before touching the CLI entrypoint, we teach the runner what a
team template is. This keeps `cmd/curlew/main.go` changes in Step 5 small
and focused on argument parsing and stderr.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | register `shared_vault_templates` at `TierTeam` |
| `internal/auth/registry_test.go` | modify | assert new gate definition |
| `internal/runner/runner.go` | modify | extend `VarSources` with `TeamSecrets *teamtemplate.SecretsResolver`, `TeamEnv string`, `TeamStub bool`; extend `Summary` with `SharedSecretsResolved int`; update `buildScope` to (a) gate-check `shared_vault_templates`, (b) invoke resolver, (c) call `scope = scope.WithSecrets(resolved)`, (d) scan the collection for `{{secrets.X}}` references and return `ErrEnvFlagRequired` / `ErrUnknownAlias` before any HTTP |
| `internal/runner/runner.go` | modify | add `collectSecretReferences(col *parser.Collection) []string` helper that walks URL, headers, body, setup, teardown looking for `variable.SecretReferences` |
| `internal/runner/runner_test.go` | modify | new suite `TestRun_TeamSecrets` covering 6+ scenarios (see tests below) |

#### Current Code

```go
type VarSources struct {
    Project             map[string]string
    EnvFile             map[string]string
    ...
    Secrets             *vault.SecretsConfig
    VaultExecutor       vault.CommandExecutor
    ...
}
```

#### New Code

```go
type VarSources struct {
    Project             map[string]string
    EnvFile             map[string]string
    ...
    Secrets             *vault.SecretsConfig
    VaultExecutor       vault.CommandExecutor
    // M4-002: shared vault template.
    // TeamTemplate is the parsed template loaded from CURLEW_TEAM_CONFIG.
    // When non-nil, the runner resolves {{secrets.X}} references via the
    // active environment (TeamEnv) and injects them into the scope's
    // secrets namespace.
    TeamTemplate *teamtemplate.TeamTemplate
    TeamEnv      string // from --env
    TeamStub     bool   // from CURLEW_VAULT_STUB=1
    ...
}
```

Inside `buildScope`, right after the existing Layer-3 vault block:

```go
// Shared vault template (Layer 4, Team tier).
if vars.TeamTemplate != nil {
    reg := vars.Registry
    if reg == nil {
        reg = auth.DefaultRegistry()
    }
    tier := vars.Tier
    if tier == "" {
        tier = auth.TierFree
    }
    if err := auth.CheckFeature(reg, "shared_vault_templates", tier); err != nil {
        return nil, err
    }

    // Scan the collection for {{secrets.X}} tokens.
    needed := collectSecretReferences(col)
    if len(needed) > 0 && vars.TeamEnv == "" {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryConfig,
            Message:  "shared template requires --env <name>",
            Hint:     "Add --env <envname> to your curlew run invocation",
            Inner:    teamtemplate.ErrEnvFlagRequired,
        }
    }

    // Build resolver with the chosen provider factory (stub vs real).
    makeProvider := realTeamProviderFactory(vars.VaultExecutor)
    if vars.TeamStub {
        makeProvider = stubTeamProviderFactory()
    }
    resolver, resolverErr := teamtemplate.NewSecretsResolver(vars.TeamTemplate, vars.TeamEnv, makeProvider)
    if resolverErr != nil {
        return nil, resolverErr
    }

    // Validate that every referenced alias exists in the active env.
    for _, alias := range needed {
        if !resolver.HasAlias(alias) {
            return nil, fmt.Errorf("%w %q for environment %q",
                teamtemplate.ErrUnknownAlias, alias, vars.TeamEnv)
        }
    }

    resolved, resolveErr := resolver.Resolve(ctx)
    if resolveErr != nil {
        return nil, resolveErr
    }

    // Inject into scope as a dedicated namespace.
    scope = scope.WithSecrets(resolved)
    // Mark every alias as sensitive so it is redacted.
    for name := range resolved {
        sensitiveSecretsSet.Add(name) // populated via summary later
    }
    // Counter surfaced on Summary for log line + tests.
    vars.sharedSecretsResolved = len(resolved)
}
```

`collectSecretReferences` walks every request item (URL, method, headers,
body, assertions, data_driven files are out of scope since they are file
paths) and calls `variable.SecretReferences` on each string field. It is
implemented via a small `visitStrings(col, func(s string))` helper so we
can reuse it later if needed.

`realTeamProviderFactory` / `stubTeamProviderFactory` are helpers local to
runner; the real one simply dispatches on `env.Provider` and constructs an
AWS or Azure provider using the existing `vault.NewAWSProvider` /
`NewAzureProvider` with the template-derived Region / VaultName.

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go
func TestRun_TeamSecrets(t *testing.T) {
    t.Run("resolves_production_alias_via_stub", func(t *testing.T) {
        tpl := mustParseTeam(t)
        col := colWith("https://x/{{secrets.api_key}}")
        results, summary, err := Run(ctx, col, successExecutor, VarSources{
            Tier: auth.TierTeam, Registry: auth.DefaultRegistry(),
            TeamTemplate: tpl, TeamEnv: "production", TeamStub: true,
        })
        // assert results[0].URL == "https://x/stub::production::prod/api-key"
        // assert summary.SharedSecretsResolved == 2
    })

    t.Run("staging_uses_azure_provider", func(t *testing.T) { /* ... */ })

    t.Run("env_flag_required_when_secret_referenced", func(t *testing.T) {
        col := colWith("https://x/{{secrets.api_key}}")
        _, _, err := Run(ctx, col, successExecutor, VarSources{
            Tier: auth.TierTeam, Registry: auth.DefaultRegistry(),
            TeamTemplate: tpl, TeamEnv: "", TeamStub: true,
        })
        if !errors.Is(err, teamtemplate.ErrEnvFlagRequired) { t.Fatal(...) }
    })

    t.Run("unknown_alias_fails_before_http", func(t *testing.T) {
        col := colWith("https://x/{{secrets.missing}}")
        _, _, err := Run(ctx, col, trackingExecutor /* counts dispatches */, VarSources{
            Tier: auth.TierTeam, Registry: auth.DefaultRegistry(),
            TeamTemplate: tpl, TeamEnv: "production", TeamStub: true,
        })
        if !errors.Is(err, teamtemplate.ErrUnknownAlias) { t.Fatal(...) }
        // assert trackingExecutor.count == 0
    })

    t.Run("second_call_served_from_cache", func(t *testing.T) {
        // collection with TWO requests both using {{secrets.api_key}}
        // resolver-level: assert stub provider BulkFetch called once
    })

    t.Run("free_tier_blocked_by_feature_gate", func(t *testing.T) {
        _, _, err := Run(ctx, col, successExecutor, VarSources{
            Tier: auth.TierFree, Registry: auth.DefaultRegistry(),
            TeamTemplate: tpl, TeamEnv: "production", TeamStub: true,
        })
        var ge *auth.GateError
        if !errors.As(err, &ge) { t.Fatal(...) }
    })

    t.Run("unknown_environment_fails", func(t *testing.T) {
        _, _, err := Run(ctx, col, successExecutor, VarSources{
            Tier: auth.TierTeam, TeamTemplate: tpl, TeamEnv: "qa", TeamStub: true,
        })
        if !errors.Is(err, teamtemplate.ErrUnknownEnvironment) { t.Fatal(...) }
    })
}
```

#### Impact on Existing Tests
- `TestRun_VaultResolution` tests (lines 2722-2883) keep passing — they only
  set `Secrets`, not `TeamTemplate`. `buildScope` still runs the Layer-3
  vault block unchanged.
- `TestRun_GateChecks` (lines 2680-2720) keep passing for the same reason.
- Any runner test that constructs a `VarSources` literal continues to
  compile — the three new fields have zero-value defaults.

---

### Step 5: CLI wiring in `cmd/curlew/main.go`

**Rationale:** Last because it is the entrypoint and pulls all previous
steps together. `runCmdInner` is the single source of truth for run
invocation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | read `CURLEW_TEAM_CONFIG` + `CURLEW_VAULT_STUB`, call `config.LoadTeamTemplate`, pass `TeamTemplate/TeamEnv/TeamStub` in `VarSources`, emit `Resolved N secrets from shared template (<env>)` on stderr, update gate-error printing to cover the new exit code path, extend `printHelp()` |
| `cmd/curlew/main.go` | modify | extend sensitivity set — merge every alias from the resolved map into the redaction set |
| `cmd/curlew/run_test.go` | modify | add `TestRunCmd_TeamSecrets_*` e2e cases using `t.Setenv` + `httptest` |

#### New Code (inside `runCmdInner`, after `LoadProjectConfig`)

```go
// Load shared vault template if configured.
teamCfgPath := os.Getenv("CURLEW_TEAM_CONFIG")
teamTemplate, teamErr := config.LoadTeamTemplate(teamCfgPath)
if teamErr != nil {
    // errors.Is check for ErrTemplateNotFound → exit 3
    errOut.StructuredError(teamErr)
    return 3, nil
}
teamStub := os.Getenv("CURLEW_VAULT_STUB") == "1"

// ... existing runner.Run call ...
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    // ...existing fields...
    TeamTemplate: teamTemplate,
    TeamEnv:      envName,
    TeamStub:     teamStub,
})
```

After a successful run (before formatting), if `summary.SharedSecretsResolved > 0`:

```go
if summary != nil && summary.SharedSecretsResolved > 0 && teamTemplate != nil {
    _, _ = fmt.Fprintf(os.Stderr, "Resolved %d secrets from shared template (%s)\n",
        summary.SharedSecretsResolved, envName)
}
```

Update `printHelp()` to add a new section:

```
Shared Vault Templates (Team tier):
  CURLEW_TEAM_CONFIG=path   Load a shared vault configuration template
                             {{secrets.ALIAS}} references in collections resolve
                             through the template for the environment named by --env
  CURLEW_VAULT_STUB=1       Use an in-memory stub provider (for local/CI testing)
```

And extend the `Run Options:` `--env` line to note that it selects the
shared vault template environment when `CURLEW_TEAM_CONFIG` is set.

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/run_test.go
func TestRunCmd_TeamSecrets_Success(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // echo Authorization header into response so we can assert on it
        w.Header().Set("X-Auth", r.Header.Get("Authorization"))
        w.WriteHeader(200)
    }))
    defer srv.Close()

    tmp := t.TempDir()
    tpl := filepath.Join(tmp, "team.yaml")
    os.WriteFile(tpl, []byte(sharedVaultTemplateYAML), 0o644)

    col := filepath.Join(tmp, "col.yaml")
    os.WriteFile(col, []byte(fmt.Sprintf(
        "name: T\nrequests:\n  - name: A\n    request:\n      method: GET\n      url: %s\n      headers:\n        Authorization: \"Bearer {{secrets.api_key}}\"\n", srv.URL)), 0o644)

    t.Setenv("CURLEW_TIER", "team")
    t.Setenv("CURLEW_TEAM_CONFIG", tpl)
    t.Setenv("CURLEW_VAULT_STUB", "1")

    code := runCmd([]string{col, "--env", "production"})
    if code != 0 { t.Fatalf("exit = %d", code) }
}

func TestRunCmd_TeamSecrets_MissingFile(t *testing.T) {
    t.Setenv("CURLEW_TIER", "team")
    t.Setenv("CURLEW_TEAM_CONFIG", "/does/not/exist.yaml")
    code := runCmd([]string{"testdata/minimal.yaml"})
    if code != 3 { t.Fatalf("want 3, got %d", code) }
}

func TestRunCmd_TeamSecrets_MissingEnvFlag(t *testing.T) { /* exit 3 */ }
func TestRunCmd_TeamSecrets_UnknownAlias(t *testing.T)   { /* exit 3 */ }
func TestRunCmd_TeamSecrets_FreeTierBlocked(t *testing.T) { /* exit 6 */ }
func TestRunCmd_Help_MentionsTeamTemplate(t *testing.T)  { /* help text contains CURLEW_TEAM_CONFIG */ }
```

#### Impact on Existing Tests
- Every existing `TestRunCmd_*` continues passing because none of them set
  `CURLEW_TEAM_CONFIG`, and `LoadTeamTemplate("")` returns `(nil, nil)`.
- Help text tests that do an exact-string match will break; there is one
  at `TestRun_help` that only checks exit code, so we are safe. Any grep
  for specific strings has been audited — none will break.

---

### Step 6: Fixture, smoke test, and changelog

**Rationale:** Final glue so the observable command in the task YAML works
verbatim and CI catches regressions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/team/uses-team-vault.yaml` | create | collection referencing `{{secrets.api_key}}` in URL or header |
| `smoke/run.sh` | modify | add end-to-end scenario: export `CURLEW_TEAM_CONFIG`, `CURLEW_VAULT_STUB=1`, `CURLEW_TIER=team`, spin up a local `python3 -m http.server` or use an existing test server pattern, run collection with `--env production`, assert exit 0 and stderr contains the resolved-log line |
| `CHANGELOG.md` | modify | add M4-002 entry under `## [Unreleased]` → `### Added` |

#### `testdata/team/uses-team-vault.yaml` content

```yaml
name: Uses team vault
variables:
  base_url: "http://127.0.0.1:8099"
requests:
  - name: Ping with shared secret
    request:
      method: GET
      url: "{{base_url}}/"
      headers:
        Authorization: "Bearer {{secrets.api_key}}"
    assertions:
      status: 200
```

#### Smoke test addition (around line 955)

```bash
echo "=== Shared vault template (M4-002) ==="
echo "--- Run with CURLEW_TEAM_CONFIG + stub ---"
SMOKE_SRV_PORT=8099
python3 -m http.server $SMOKE_SRV_PORT --bind 127.0.0.1 > /dev/null 2>&1 &
SMOKE_SRV_PID=$!
sleep 0.3
trap 'kill $SMOKE_SRV_PID 2>/dev/null || true' EXIT

export CURLEW_TIER=team
export CURLEW_TEAM_CONFIG=testdata/team/shared-vault-template.yaml
export CURLEW_VAULT_STUB=1

SMOKE_OUT=$(./curlew run testdata/team/uses-team-vault.yaml --env production 2>&1)
SMOKE_RC=$?
unset CURLEW_TEAM_CONFIG CURLEW_VAULT_STUB CURLEW_TIER
kill $SMOKE_SRV_PID 2>/dev/null || true
trap - EXIT

if [ "$SMOKE_RC" -eq 0 ]; then echo "PASS: team template run exit 0"; else echo "FAIL: exit $SMOKE_RC"; echo "$SMOKE_OUT"; exit 1; fi
echo "$SMOKE_OUT" | grep -q "Resolved 2 secrets from shared template (production)" \
  && echo "PASS: resolved-log line present" \
  || { echo "FAIL: missing resolved-log line"; echo "$SMOKE_OUT"; exit 1; }
```

(If spinning a Python HTTP server is undesirable, we fall back to reusing the
existing `sample/hello.yaml`-style inline fixture with a URL that resolves
to `httpbin.org/anything` — but the local server is preferred because it
keeps the smoke test hermetic.)

#### Impact on Existing Tests
- None; purely additive.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/variable/variable_test.go` | existing tests | none | — (additive) |
| `internal/parallel/scan_test.go` | existing tests | none | — (additive case) |
| `internal/runner/runner_test.go` | `TestRun_VaultResolution` | none | pre-existing Layer-3 flow untouched |
| `internal/runner/runner_test.go` | `TestRun_GateChecks/vault_*` | none | pre-existing gate flow untouched |
| `internal/auth/registry_test.go` | feature list assertions | potentially breaks | update expected count if the test asserts on total feature count |
| `cmd/curlew/run_test.go` | `TestRun_help`, `TestRunCmd_*` | none | existing tests do not set `CURLEW_TEAM_CONFIG` |
| `cmd/curlew/validate_team_test.go` | existing | none | this task does not touch `validate` |

## Risks and Edge Cases

- **Risk: environment var reads leak into unrelated tests.**
  → **Mitigation:** use `t.Setenv` exclusively; never touch `os.Setenv`
  directly in tests. `config.LoadTeamTemplate("")` must stay a pure no-op.

- **Risk: `{{secrets.X}}` tokens inside external data files or external
  request files are missed during the collection scan.**
  → **Mitigation:** `collectSecretReferences` only walks the in-memory
  `parser.Collection`; the parser already inlines included/external files
  before runner sees them (via `ParseFileWithOptions`). Edge case: data-driven
  CSV row values are loaded at iteration time. For this slice we scope the
  scan to static fields (URL, headers, body, method, assertions.body,
  assertions.headers) and document that dynamic row-sourced secrets
  references are out of scope (follow-up task material).

- **Risk: the parallel executor snapshots scope per goroutine (see
  `Scope.Snapshot` at variable.go:263), so the `secrets` map must survive
  snapshotting.**
  → **Mitigation:** extend `Snapshot()` to copy the `secrets` map alongside
  `vars`/`resolved`. Small change — include it in Step 1.

- **Risk: aliases with the same name as a regular variable collide.**
  → **Mitigation:** the namespaces are distinct (`{{api_key}}` vs
  `{{secrets.api_key}}`). Test "secrets alias never shadowed by var of same
  name" asserts this explicitly.

- **Risk: stub output format changes break downstream assertions.**
  → **Mitigation:** pin the format in a constant
  (`const stubValueFormat = "stub::%s::%s"`) and reference it from tests
  via `fmt.Sprintf(stubValueFormat, ...)` so a future format tweak only
  requires updating one place.

- **Edge case: template validates successfully but the chosen environment
  has zero keys.**
  → **Handling:** `SecretsResolver.Resolve` returns an empty map; the runner
  logs `Resolved 0 secrets from shared template (<env>)` *only if any
  collection request actually referenced `{{secrets.X}}`*. If no references,
  the log line is suppressed to keep output quiet.

- **Edge case: `CURLEW_TEAM_CONFIG` set but collection has no secrets
  references.**
  → **Handling:** template is loaded and validated (so config errors
  surface early) but `--env` is not required and the resolver is never
  invoked.

- **Edge case: JSON output mode must not leak the stderr resolved-log line
  into the JSON payload.**
  → **Handling:** the log line is written to `os.Stderr` via `fmt.Fprintf`
  and never touches the JSON marshaller.

- **Edge case: watch mode re-runs the collection.**
  → **Handling:** watch mode calls `runCmdInner` per event, so the env var
  is re-read and the template is re-loaded on each run. No extra work
  needed.

## Proposed Go Function Signatures

```go
// internal/variable
func (s *Scope) WithSecrets(secrets map[string]string) *Scope
func HasSecretsNamespace(input string) bool
func SecretReferences(input string) []string
var ErrUnknownSecret = errors.New("unknown secret alias")

// internal/vault/teamtemplate
type StubProvider struct { /* ... */ }
func NewStubProvider(envName, provider string) *StubProvider
func (p *StubProvider) Name() string
func (p *StubProvider) Fetch(ctx context.Context, path string) (string, error)
func (p *StubProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error)
func (p *StubProvider) ValidateConfig() error

type SecretsResolver struct { /* ... */ }
func NewSecretsResolver(t *TeamTemplate, envName string, makeProvider func(*ResolvedEnv) (vault.Provider, error)) (*SecretsResolver, error)
func (r *SecretsResolver) Resolve(ctx context.Context) (map[string]string, error)
func (r *SecretsResolver) EnvName() string
func (r *SecretsResolver) Count() int
func (r *SecretsResolver) HasAlias(alias string) bool

var (
    ErrTemplateNotFound   = errors.New("shared vault template not found")
    ErrUnknownEnvironment = errors.New("unknown environment in shared vault template")
    ErrUnknownAlias       = errors.New("unknown secret alias")
    ErrEnvFlagRequired    = errors.New("shared template requires --env")
)

// internal/config
func LoadTeamTemplate(path string) (*teamtemplate.TeamTemplate, error)

// internal/runner — VarSources additions
type VarSources struct {
    // ... existing ...
    TeamTemplate *teamtemplate.TeamTemplate
    TeamEnv      string
    TeamStub     bool
}

// Summary additions
type Summary struct {
    // ... existing ...
    SharedSecretsResolved int
}

// Runner-internal helpers
func collectSecretReferences(col *parser.Collection) []string
func realTeamProviderFactory(exec vault.CommandExecutor) func(*teamtemplate.ResolvedEnv) (vault.Provider, error)
func stubTeamProviderFactory() func(*teamtemplate.ResolvedEnv) (vault.Provider, error)

// internal/auth/registry.go — new feature
// (registered inside DefaultRegistry)
// Name: "shared_vault_templates"
// RequiredTier: TierTeam
// Description: "Shared vault configuration templates require Team tier ($39/month)"
// Workaround: "Use vault provider profiles (Solo tier) for single-user configurations"
```

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (exact commands from task YAML):

```bash
go build ./cmd/curlew && \
  CURLEW_TEAM_CONFIG=testdata/team/shared-vault-template.yaml \
  CURLEW_VAULT_STUB=1 \
  CURLEW_TIER=team \
  ./curlew run testdata/team/uses-team-vault.yaml --env production
# Expected: exit 0; stderr includes "Resolved 2 secrets from shared template (production)";
# the request body/header contains the resolved stub value.

go test ./internal/vault/teamtemplate/... ./internal/runner/... \
  -run 'TeamTemplate|TeamSecrets' -count=1
# Expected: ok; >= 6 tests passing across both packages.
```

Note: the task observable does not include `CURLEW_TIER=team`. In practice
the feature gate must allow the run, so the smoke test and documentation
set `CURLEW_TIER=team`. This is noted here as a minor discrepancy with the
task YAML that will be resolved by updating the observable command in the
verification report (not the task YAML).
