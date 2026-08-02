# Implementation Plan: M2-008

## Overview
Extend Curlew to support `auth_profiles:` in `curlew.yaml`, executing dynamic auth collections before main requests and injecting extracted variables into pre-execution scope.

## Task Details
- **ID:** M2-008
- **Title:** Auth profile configuration and execution
- **Phase:** M2: Dynamic Auth
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-016 | Variable extraction from responses | done |
| M1-017 | Sensitive variable handling | done |
| M1-028 | Feature gate integration | done |

## Implementation Steps

### Step 1: Register `dynamic_auth_profiles` feature gate
**Rationale:** Smallest blast radius — purely additive. No existing code changes required. Must exist before any gate check in Step 6.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `dynamic_auth_profiles` (Solo tier) to `DefaultRegistry()` |
| `internal/auth/registry_test.go` | modify | Add table entry for new gate |

#### Current Code
```go
func DefaultRegistry() *Registry {
	r := &Registry{}
	r.Register(FeatureDefinition{
		Name:         "vault_provider_profiles",
		RequiredTier: TierSolo,
		Description:  "Vault provider profiles require Solo tier ($9/month)",
		Workaround:   "Use --var to inject secrets manually",
	})
	r.Register(FeatureDefinition{
		Name:         "from_command",
		RequiredTier: TierSolo,
		Description:  "from_command requires Solo tier ($9/month)",
		Workaround:   "Use --var to inject values manually",
	})
	return r
}
```

#### New Code
```go
func DefaultRegistry() *Registry {
	r := &Registry{}
	r.Register(FeatureDefinition{
		Name:         "vault_provider_profiles",
		RequiredTier: TierSolo,
		Description:  "Vault provider profiles require Solo tier ($9/month)",
		Workaround:   "Use --var to inject secrets manually",
	})
	r.Register(FeatureDefinition{
		Name:         "from_command",
		RequiredTier: TierSolo,
		Description:  "from_command requires Solo tier ($9/month)",
		Workaround:   "Use --var to inject values manually",
	})
	r.Register(FeatureDefinition{
		Name:         "dynamic_auth_profiles",
		RequiredTier: TierSolo,
		Description:  "Dynamic auth profiles require Solo tier ($9/month)",
		Workaround:   "Use --var to inject auth tokens manually, or use variables with from_command",
	})
	return r
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry(t *testing.T) {
    tests := []struct {
        name    string
        feature string
        want    bool
    }{
        {"contains vault_provider_profiles", "vault_provider_profiles", true},
        {"contains from_command", "from_command", true},
        {"contains dynamic_auth_profiles", "dynamic_auth_profiles", true},  // NEW
        {"unknown feature absent", "nonexistent_feature", false},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests break — purely additive.

---

### Step 2: Define auth profile types (`internal/auth/profile.go` — new file)
**Rationale:** Establishes data model before any parsing or execution logic. Fully isolated — no existing file changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/profile.go` | create | Profile type, sentinel errors, ExecuteFunc callback type, ProfileResult |
| `internal/auth/profile_test.go` | create | Table-driven tests for ExecuteProfiles |

#### New Code
```go
package auth

import (
    "context"
    "errors"
    "fmt"
    "path/filepath"

    "github.com/weiqigod/curlew/internal/variable"
)

// Sentinel errors for auth profile failures.
var (
    ErrProfileFailed   = errors.New("auth profile execution failed")
    ErrProfileNotFound = errors.New("auth profile collection not found")
    ErrProfileNoExtract = errors.New("auth profile produced no variables")
)

// ProfileType identifies the kind of auth profile.
type ProfileType string

const ProfileDynamic ProfileType = "dynamic"

// Profile represents a single auth profile entry from curlew.yaml.
type Profile struct {
    Name       string      // profile key name (e.g., "admin_token")
    Type       ProfileType // "dynamic"
    Collection string      // relative path to collection file
    Extract    string      // variable name to extract (empty = all extracted vars)
}

// ProfileResult holds the outcome of executing all auth profiles.
type ProfileResult struct {
    Variables map[string]string
    Sensitive *variable.SensitiveSet
}

// ExecuteFunc executes a collection at the given path and returns extracted variables.
type ExecuteFunc func(ctx context.Context, collectionPath string) (map[string]string, error)

// ExecuteProfiles runs all auth profiles sequentially and returns merged variables.
// All returned variables are automatically marked as sensitive.
// Returns ErrProfileFailed if any profile execution fails.
func ExecuteProfiles(ctx context.Context, profiles []Profile, projectRoot string, execute ExecuteFunc) (*ProfileResult, error) {
    result := &ProfileResult{
        Variables: make(map[string]string),
        Sensitive: variable.NewSensitiveSet(),
    }
    if len(profiles) == 0 {
        return result, nil
    }

    for _, p := range profiles {
        collPath := p.Collection
        if projectRoot != "" && !filepath.IsAbs(collPath) {
            collPath = filepath.Join(projectRoot, collPath)
        }

        vars, err := execute(ctx, collPath)
        if err != nil {
            return nil, fmt.Errorf("auth profile %q: %w: %w", p.Name, ErrProfileFailed, err)
        }

        if p.Extract != "" {
            val, ok := vars[p.Extract]
            if !ok {
                return nil, fmt.Errorf("auth profile %q: %w: variable %q not found in collection output",
                    p.Name, ErrProfileNoExtract, p.Extract)
            }
            result.Variables[p.Extract] = val
            result.Sensitive.Add(p.Extract)
        } else {
            for k, v := range vars {
                result.Variables[k] = v
                result.Sensitive.Add(k)
            }
        }
    }

    return result, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
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
            execute: func(_ context.Context, path string) (map[string]string, error) {
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
            result, err := ExecuteProfiles(ctx, tt.profiles, tt.projectRoot, tt.execute)
            if tt.wantErr != nil {
                if !errors.Is(err, tt.wantErr) {
                    t.Errorf("got err %v, want %v", err, tt.wantErr)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if diff := cmp.Diff(tt.wantVars, result.Variables); diff != "" {
                t.Errorf("variables mismatch (-want +got):\n%s", diff)
            }
            // Verify all returned variables are sensitive
            for k := range result.Variables {
                if !result.Sensitive.Contains(k) {
                    t.Errorf("variable %q not marked sensitive", k)
                }
            }
        })
    }
}
```

#### Impact on Existing Tests
- None. New file only.

---

### Step 3: Add `Resolved()` method to `variable.Scope`
**Rationale:** Needed by `RunForExtraction` (Step 5) to diff scope before/after auth collection execution. Purely additive to the variable package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add `Resolved() map[string]string` method to `Scope` |
| `internal/variable/variable_test.go` | modify | Add `TestScope_Resolved` |

#### New Code
```go
// Resolved returns a copy of all currently resolved variable values.
func (s *Scope) Resolved() map[string]string {
    out := make(map[string]string, len(s.resolved))
    for k, v := range s.resolved {
        out[k] = v
    }
    return out
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestScope_Resolved(t *testing.T) {
    s := NewScope(map[string]string{"key1": "val1", "key2": "val2"})
    got := s.Resolved()
    want := map[string]string{"key1": "val1", "key2": "val2"}
    if diff := cmp.Diff(want, got); diff != "" {
        t.Errorf("Resolved() mismatch (-want +got):\n%s", diff)
    }
    // Mutating the returned map should not affect the scope
    got["key1"] = "mutated"
    if s.Resolved()["key1"] != "val1" {
        t.Error("Resolved() returned a non-copy")
    }
}
```

#### Impact on Existing Tests
- None. Additive method.

---

### Step 4: Extend `ProjectConfig` to parse `auth_profiles:`
**Rationale:** Config parsing is independent of execution. Backward-compatible (new field is optional).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add `AuthProfiles []auth.Profile` to `ProjectConfig`; parse `auth_profiles:` YAML block |
| `internal/config/project_test.go` | modify | Add `TestParseProjectConfig_AuthProfiles` |

#### Current Code
```go
type ProjectConfig struct {
    ProjectName string
    Variables   map[string]string
    Secrets     *vault.SecretsConfig
}

type projectFile struct {
    Project   string            `yaml:"project"`
    Variables map[string]string `yaml:"variables"`
    Secrets   yaml.Node         `yaml:"secrets"`
}
```

#### New Code
```go
type ProjectConfig struct {
    ProjectName  string
    Variables    map[string]string
    Secrets      *vault.SecretsConfig
    AuthProfiles []auth.Profile  // parsed from auth_profiles: block
}

type projectFile struct {
    Project      string            `yaml:"project"`
    Variables    map[string]string `yaml:"variables"`
    Secrets      yaml.Node         `yaml:"secrets"`
    AuthProfiles yaml.Node         `yaml:"auth_profiles"`
}

type authProfileEntry struct {
    Type       string `yaml:"type"`
    Collection string `yaml:"collection"`
    Extract    string `yaml:"extract,omitempty"`
}
```

Parsing logic added to `ParseProjectConfig()`:
```go
if pf.AuthProfiles.Kind != 0 {
    var raw map[string]authProfileEntry
    if err := pf.AuthProfiles.Decode(&raw); err != nil {
        return nil, fmt.Errorf("%w: auth_profiles: %s", ErrInvalidProjectConfig, err)
    }
    for name, entry := range raw {
        if entry.Type != string(auth.ProfileDynamic) {
            return nil, fmt.Errorf("%w: auth_profiles: unsupported type %q for profile %q",
                ErrInvalidProjectConfig, entry.Type, name)
        }
        if entry.Collection == "" {
            return nil, fmt.Errorf("%w: auth_profiles: profile %q missing collection path",
                ErrInvalidProjectConfig, name)
        }
        cfg.AuthProfiles = append(cfg.AuthProfiles, auth.Profile{
            Name:       name,
            Type:       auth.ProfileDynamic,
            Collection: entry.Collection,
            Extract:    entry.Extract,
        })
    }
    sort.Slice(cfg.AuthProfiles, func(i, j int) bool {
        return cfg.AuthProfiles[i].Name < cfg.AuthProfiles[j].Name
    })
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseProjectConfig_AuthProfiles(t *testing.T) {
    tests := []struct {
        name       string
        yaml       string
        wantLen    int
        wantProfile auth.Profile
        wantErr    bool
    }{
        {
            name: "no auth_profiles block",
            yaml: "project: test\n",
            wantLen: 0,
        },
        {
            name: "dynamic profile with extract",
            yaml: `project: test
auth_profiles:
  admin_token:
    type: dynamic
    collection: auth/login.yaml
    extract: admin_token
`,
            wantLen: 1,
            wantProfile: auth.Profile{
                Name: "admin_token", Type: auth.ProfileDynamic,
                Collection: "auth/login.yaml", Extract: "admin_token",
            },
        },
        {
            name: "dynamic profile without extract (all vars)",
            yaml: `project: test
auth_profiles:
  login:
    type: dynamic
    collection: auth/login.yaml
`,
            wantLen: 1,
            wantProfile: auth.Profile{
                Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml",
            },
        },
        {
            name: "unsupported type returns error",
            yaml: `project: test
auth_profiles:
  login:
    type: static
    collection: auth/login.yaml
`,
            wantErr: true,
        },
        {
            name: "missing collection returns error",
            yaml: `project: test
auth_profiles:
  login:
    type: dynamic
`,
            wantErr: true,
        },
        {
            name: "multiple profiles sorted by name",
            yaml: `project: test
auth_profiles:
  zebra:
    type: dynamic
    collection: auth/zebra.yaml
  alpha:
    type: dynamic
    collection: auth/alpha.yaml
`,
            wantLen: 2,
            // first profile should be "alpha"
        },
    }
    // ...
}
```

#### Impact on Existing Tests
- No break. New field is optional; existing YAML without `auth_profiles:` leaves `AuthProfiles` nil.

---

### Step 5: Add `RunForExtraction` and wire auth profiles into `runner.Run()`
**Rationale:** Core integration. Guarded by non-empty `AuthProfiles` slice so existing behavior is unchanged when no profiles are configured.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `AuthProfiles []auth.Profile` + `ProjectRoot string` to `VarSources`; add `RunForExtraction`; wire auth execution before setup phase |
| `internal/runner/runner_test.go` | modify | Add auth profile test cases |

#### New Fields in `VarSources`
```go
type VarSources struct {
    // ... existing fields unchanged ...
    AuthProfiles []auth.Profile // from curlew.yaml
    ProjectRoot  string         // for resolving relative collection paths
}
```

#### New Function
```go
// RunForExtraction executes a collection and returns variables set during execution.
// Requests do NOT count toward the caller's guard rail counter.
// AuthProfiles is intentionally not forwarded to prevent recursive auth execution.
func RunForExtraction(ctx context.Context, collectionPath string, exec ExecuteFunc, vars VarSources) (map[string]string, error) {
    col, err := parser.ParseFile(collectionPath)
    if err != nil {
        return nil, fmt.Errorf("auth collection: %w", err)
    }

    // Shallow copy vars without auth profiles to prevent recursion
    innerVars := vars
    innerVars.AuthProfiles = nil
    innerVars.ProjectRoot = ""

    // Run collection; uses separate guard rail counter
    _, summary, err := Run(ctx, col, exec, innerVars)
    if err != nil {
        return nil, err
    }

    // Check for request failures
    for _, r := range summary {
        if r.Err != nil {
            return nil, fmt.Errorf("auth request %q: %w", r.Name, r.Err)
        }
        if r.Assertions != nil && !r.Assertions.Passed {
            return nil, fmt.Errorf("auth request %q: assertions failed", r.Name)
        }
    }

    return summary.ExtractedVars(), nil
}
```

> **Note:** `summary.ExtractedVars()` requires knowing what was extracted. See alternative below.

**Alternative approach using Scope diff** (preferred for clean separation):

```go
func RunForExtraction(ctx context.Context, collectionPath string, exec ExecuteFunc, vars VarSources) (map[string]string, error) {
    col, err := parser.ParseFile(collectionPath)
    if err != nil {
        return nil, fmt.Errorf("auth collection: %w", err)
    }

    // Build scope the same way Run() does, capture initial state
    scope := buildScope(ctx, col, vars)   // extracted from Run()
    before := scope.Resolved()

    innerVars := vars
    innerVars.AuthProfiles = nil
    innerVars.ProjectRoot = ""

    results, _, runErr := runPhases(ctx, col, exec, scope)
    if runErr != nil {
        return nil, runErr
    }
    for _, r := range results {
        if r.Err != nil {
            return nil, fmt.Errorf("auth request %q: %w", r.Name, r.Err)
        }
    }

    after := scope.Resolved()
    extracted := make(map[string]string)
    for k, v := range after {
        if old, ok := before[k]; !ok || old != v {
            extracted[k] = v
        }
    }
    return extracted, nil
}
```

> **Implementation note:** During execution, we will determine which approach is cleaner based on what `Run()` actually returns (likely `[]RequestResult`). The scope diff approach is preferred as it avoids coupling to result structs.

#### Wiring in `Run()` — insert after scope resolution, before Phase 1 (setup):
```go
// Auth profile execution (Solo tier feature, before all phases)
if len(vars.AuthProfiles) > 0 {
    reg := vars.Registry
    if reg == nil {
        reg = auth.DefaultRegistry()
    }
    tier := vars.Tier
    if tier == "" {
        tier = auth.TierFree
    }
    if err := auth.CheckFeature(reg, "dynamic_auth_profiles", tier); err != nil {
        return nil, summary, err
    }

    authExec := func(ctx context.Context, collPath string) (map[string]string, error) {
        return RunForExtraction(ctx, collPath, exec, vars)
    }
    profileResult, authErr := auth.ExecuteProfiles(ctx, vars.AuthProfiles, vars.ProjectRoot, authExec)
    if authErr != nil {
        return nil, summary, fmt.Errorf("auth profile: %w", authErr)
    }
    for k, v := range profileResult.Variables {
        scope.Set(k, v)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_AuthProfiles(t *testing.T) {
    tests := []struct {
        name         string
        authProfiles []auth.Profile
        tier         auth.Tier
        requests     []*parser.Request
        wantErr      bool
        wantGateErr  bool
        wantSkipped  bool
    }{
        {
            name:        "no auth profiles - unchanged behavior",
            tier:        auth.TierFree,
            wantErr:     false,
        },
        {
            name: "auth profile gate blocks free tier",
            authProfiles: []auth.Profile{
                {Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
            },
            tier:        auth.TierFree,
            wantGateErr: true,
        },
        {
            name: "auth profile variables available in main requests",
            authProfiles: []auth.Profile{
                {Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml", Extract: "token"},
            },
            tier: auth.TierSolo,
            // main request uses {{token}} which must resolve
        },
        {
            name: "auth profile failure causes exit code 5 path",
            authProfiles: []auth.Profile{
                {Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
            },
            tier:    auth.TierSolo,
            wantErr: true,
        },
        {
            name: "auth profile requests do not count toward guard rail",
            authProfiles: []auth.Profile{
                {Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
            },
            tier: auth.TierSolo,
            // counter must be 0 after auth profile execution, not incremented
        },
    }
    // ...
}
```

#### Impact on Existing Tests
- `VarSources` gets two new zero-value fields — existing test structs compile unchanged.
- `Run()` behavior is unchanged when `AuthProfiles` is nil/empty.
- No existing test breaks.

---

### Step 6: Wire auth profiles in `cmd/curlew/main.go`
**Rationale:** Final plumbing — passes parsed config into runner. Also updates help text.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Pass `AuthProfiles` + `ProjectRoot` to `VarSources`; update help text |

#### Current Code (VarSources construction in runCmd)
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    Project:       projectCfg.Variables,
    EnvFile:       envVars,
    // ...
})
```

#### New Code
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    Project:       projectCfg.Variables,
    EnvFile:       envVars,
    // ... existing fields ...
    AuthProfiles:  projectCfg.AuthProfiles,
    ProjectRoot:   projectRoot,  // directory containing curlew.yaml
})
```

Also update `curlew.yaml` example in help text to document the `auth_profiles:` block:

```
auth_profiles:
  admin_token:
    type: dynamic
    collection: auth/get_admin_token.yaml
    extract: admin_token   # optional: extract a specific variable
```

#### Impact on Existing Tests
- No test changes required. `main.go` wiring is typically covered by integration/smoke tests.

---

### Step 7: Update smoke test for auth profile observable
**Rationale:** Confirms end-to-end observable output works.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add auth profile scenario (gate error path for Free tier) |

The smoke test adds:
1. Create temp `curlew.yaml` with `auth_profiles:` block
2. Run `curlew run collection.yaml` and verify exit code 6 (gate blocked on Free tier)
3. This verifies the gate check fires correctly without requiring a live HTTP server

```bash
# Smoke: auth_profiles gate check (Free tier → exit 6)
cat > "$TMPDIR/curlew.yaml" <<'YAML'
project: smoke-auth
auth_profiles:
  login:
    type: dynamic
    collection: auth/login.yaml
YAML
curlew run "$TMPDIR/collection.yaml" 2>&1 | grep -q "dynamic_auth_profiles"
assert_exit 6 "auth profile gate blocked on Free tier"
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | needs new case | add `dynamic_auth_profiles` entry |
| `internal/auth/profile_test.go` | `TestExecuteProfiles` | new | create file |
| `internal/config/project_test.go` | `TestParseProjectConfig_AuthProfiles` | new | add test function |
| `internal/variable/variable_test.go` | `TestScope_Resolved` | new | add test function |
| `internal/runner/runner_test.go` | `TestRun_AuthProfiles` | new | add test functions |
| All other existing tests | — | none | no action needed |

## Risks and Edge Cases

- **Risk:** Recursive auth profiles (auth collection references auth_profiles) → **Mitigation:** `VarSources.AuthProfiles = nil` in `RunForExtraction` prevents recursion; document this behavior.
- **Risk:** Auth collection variable naming collision with main collection → **Mitigation:** Auth profile vars injected via `scope.Set()` after initial resolution; same precedence rules apply. Document that CLI/env-var overrides auth profile vars.
- **Risk:** YAML map ordering for multiple profiles → **Mitigation:** Sort profiles by name alphabetically in `ParseProjectConfig` for deterministic execution order.
- **Risk:** Guard rail bypass via auth profile with 1000+ requests → **Mitigation:** Auth profile collection has its own internal guard rail counter; still limited to 1,000 requests internally.
- **Edge case:** Auth collection file does not exist → `parser.ParseFile` returns wrapped error → `RunForExtraction` returns error → runner maps to exit code 5.
- **Edge case:** Auth profile variables marked sensitive in main output → Wire `ProfileResult.Sensitive` into the main output's sensitive set in `main.go`.
- **Edge case:** Auth profile execution times out → `context.Context` propagation handles this; no special code needed.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Add auth_profiles: to curlew.yaml with a dynamic profile referencing a login collection.
# Run curlew run tests.yaml and confirm the auth profile executes before main requests.
go test ./internal/auth/...
```
