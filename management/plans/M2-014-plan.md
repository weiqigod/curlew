# Implementation Plan: M2-014

## Overview
Add retry configuration merging with full precedence chain: built-in defaults < global (apitest.yaml) < collection < section (setup/requests/teardown) < request. Implement deep merge rules per spec: scalars replaced, arrays replaced entirely, objects deep-merged.

## Task Details
- **ID:** M2-014
- **Title:** Retry configuration precedence (global, collection, section, request)
- **Phase:** M2: Retries
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-013 | Basic retry logic with configurable max attempts | done |

## Precedence Order (lowest → highest)

Per spec `docs/SPECIFICATION.md` §Configuration Precedence and Inheritance:

1. **Built-in defaults** — hardcoded in `retry.BuiltinDefaults()`
2. **Global config** — `apitest.yaml` `defaults.retry:` block
3. **Collection-level** — top-level `retry:` in collection YAML
4. **Section-level** — `setup.retry:` / `requests.retry:` / `teardown.retry:`
5. **Request-level** — per-request `retry:` override (highest)

Merge rules:
- **Scalar fields** (booleans, numbers, strings): higher replaces lower
- **Array fields** (`status_codes`, `methods`, `status_ranges`): higher replaces entire array
- **Object fields** (`retry_on`, `do_not_retry_on`): deep merge — sub-fields individually follow scalar/array rules, omitted sub-fields inherit

## Implementation Steps

### Step 1: Add FullConfig type and merge logic to `internal/retry/`
**Rationale:** Smallest blast radius — adds new types and functions without changing any existing code. All subsequent steps depend on this.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/config.go` | modify | Add `FullConfig`, `RetryOnConfig`, `DoNotRetryOnConfig` types with pointer fields; add `BuiltinDefaults()`, pointer helpers |
| `internal/retry/merge.go` | create | `MergeConfigs(base, overlay)` and `MergeAll(configs...)`, `(FullConfig).Resolve() Config` |
| `internal/retry/merge_test.go` | create | Table-driven tests for all merge behaviors |

#### Current Code (`internal/retry/config.go`)
```go
type Config struct {
    Enabled        bool `yaml:"enabled"`
    MaxAttempts    int  `yaml:"max_attempts"`
    InitialDelayMs int  `yaml:"initial_delay_ms"`
}
```

#### New Types (added to `config.go`)
```go
// RetryOnConfig controls which conditions trigger retries.
type RetryOnConfig struct {
    StatusCodes   []int    `yaml:"status_codes,omitempty"`
    StatusRanges  []string `yaml:"status_ranges,omitempty"`
    NetworkErrors *bool    `yaml:"network_errors,omitempty"`
    Timeouts      *bool    `yaml:"timeouts,omitempty"`
    Methods       []string `yaml:"methods,omitempty"`
}

// DoNotRetryOnConfig controls exclusions from retry.
type DoNotRetryOnConfig struct {
    StatusCodes  []int    `yaml:"status_codes,omitempty"`
    StatusRanges []string `yaml:"status_ranges,omitempty"`
    Methods      []string `yaml:"methods,omitempty"`
}

// FullConfig represents retry configuration with pointer fields for merge semantics.
// Nil fields mean "not set" (inherit from lower precedence).
type FullConfig struct {
    Enabled           *bool               `yaml:"enabled,omitempty"`
    MaxAttempts       *int                `yaml:"max_attempts,omitempty"`
    BackoffStrategy   *string             `yaml:"backoff_strategy,omitempty"`
    InitialDelayMs    *int                `yaml:"initial_delay_ms,omitempty"`
    MaxDelayMs        *int                `yaml:"max_delay_ms,omitempty"`
    Jitter            *bool               `yaml:"jitter,omitempty"`
    JitterFactor      *float64            `yaml:"jitter_factor,omitempty"`
    RespectRetryAfter *bool               `yaml:"respect_retry_after,omitempty"`
    RetryOn           *RetryOnConfig      `yaml:"retry_on,omitempty"`
    DoNotRetryOn      *DoNotRetryOnConfig `yaml:"do_not_retry_on,omitempty"`
}

func BoolPtr(v bool) *bool        { return &v }
func IntPtr(v int) *int           { return &v }
func Float64Ptr(v float64) *float64 { return &v }
func StringPtr(v string) *string  { return &v }

func BuiltinDefaults() FullConfig { /* spec defaults */ }
```

#### New Code (`internal/retry/merge.go`)
```go
// MergeConfigs merges overlay on top of base. Non-nil fields in overlay replace base.
// Object fields (RetryOn, DoNotRetryOn) are deep-merged: non-nil sub-fields replace.
// Array fields within objects replace entirely (not appended).
func MergeConfigs(base, overlay FullConfig) FullConfig { ... }

// MergeAll merges configs left-to-right (later = higher precedence).
func MergeAll(configs ...FullConfig) FullConfig { ... }

// Resolve converts a fully-merged FullConfig into the concrete Config used by ExecuteWithRetry.
func (fc FullConfig) Resolve() Config { ... }
```

#### Tests to Write FIRST (RED phase)

```go
// internal/retry/merge_test.go
func TestMergeConfigs(t *testing.T) {
    tests := []struct {
        name    string
        base    FullConfig
        overlay FullConfig
        check   func(t *testing.T, result FullConfig)
    }{
        {"scalar override - enabled replaces", ...},
        {"scalar override - max_attempts replaces", ...},
        {"nil overlay field inherits base", ...},
        {"array field replaces entirely - status_codes", ...},
        {"array field replaces entirely - methods", ...},
        {"object deep merge - retry_on sub-fields", ...},
        {"object deep merge - inherits unset sub-fields", ...},
        {"nil overlay retains base completely", ...},
        {"both nil results in zero value", ...},
    }
}

func TestMergeAll(t *testing.T) {
    tests := []struct {
        name    string
        configs []FullConfig
        check   func(t *testing.T, result FullConfig)
    }{
        {"empty input returns zero FullConfig", ...},
        {"single config returned as-is", ...},
        {"three levels merged left to right", ...},
        {"global false + collection true = enabled", ...},  // Behavior 1
        {"collection max 5 + request max 10 = 10", ...},    // Behavior 2
        {"global status_codes replaced by collection", ...}, // Behavior 5
        {"global network_errors inherited when collection omits", ...}, // Behavior 6
    }
}

func TestBuiltinDefaults(t *testing.T) {
    // Verify spec defaults: enabled=false, max_attempts=3, exponential, etc.
}

func TestFullConfig_Resolve(t *testing.T) {
    tests := []struct {
        name string
        fc   FullConfig
        want Config
    }{
        {"fully populated", ...},
        {"nil fields use Config zero values", ...},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (all new types/functions)

---

### Step 2: Add Section type to parser with custom UnmarshalYAML
**Rationale:** Parser changes must come before runner changes. The Section type enables section-level retry config. This step has the largest blast radius (122 references across 6 files) but is purely mechanical for existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Section` type, update `Collection` fields, update `RetryConfig` → use `retry.FullConfig`, change `RequestItem.Retry` type |
| `internal/parser/parser.go` | modify | Update references to `col.Setup` → `col.Setup.Items` etc. |
| `internal/parser/parser_test.go` | modify | Update all Collection constructions and field accesses |
| `internal/parser/external.go` | modify | Update references if any |
| `internal/parser/external_test.go` | modify | Update Collection constructions |
| `internal/parser/testdata/with_section_retry.yaml` | create | Test data for section-level retry |

#### Current Code
```go
type Collection struct {
    Name          string        `yaml:"name"`
    Retry         RetryConfig   `yaml:"retry,omitempty"`
    Setup         []RequestItem `yaml:"setup,omitempty"`
    Requests      []RequestItem `yaml:"requests"`
    Teardown      []RequestItem `yaml:"teardown,omitempty"`
    ...
}

type RetryConfig struct {
    Enabled        bool `yaml:"enabled"`
    MaxAttempts    int  `yaml:"max_attempts"`
    InitialDelayMs int  `yaml:"initial_delay_ms"`
}
```

#### New Code
```go
// Section represents a phase (setup, requests, teardown) with optional retry config.
// Supports two YAML forms:
//   - Array form: setup: [{name: ..., request: ...}]
//   - Object form: setup: { retry: {...}, items: [{name: ..., request: ...}] }
type Section struct {
    Retry *retry.FullConfig `yaml:"retry,omitempty"`
    Items []RequestItem     `yaml:"items,omitempty"`
}

// UnmarshalYAML handles both array and object forms.
func (s *Section) UnmarshalYAML(value *yaml.Node) error {
    switch value.Kind {
    case yaml.SequenceNode:
        return value.Decode(&s.Items)
    case yaml.MappingNode:
        type sectionRaw struct {
            Retry *retry.FullConfig `yaml:"retry,omitempty"`
            Items []RequestItem     `yaml:"items,omitempty"`
        }
        var raw sectionRaw
        if err := value.Decode(&raw); err != nil {
            return err
        }
        s.Retry = raw.Retry
        s.Items = raw.Items
        return nil
    default:
        return fmt.Errorf("section: expected sequence or mapping, got %v", value.Kind)
    }
}

type Collection struct {
    Name          string            `yaml:"name"`
    Description   string            `yaml:"description,omitempty"`
    Variables     SensitiveVars     `yaml:"variables,omitempty"`
    Retry         *retry.FullConfig `yaml:"retry,omitempty"`   // collection-level
    Setup         Section           `yaml:"setup,omitempty"`   // was []RequestItem
    Requests      Section           `yaml:"requests"`          // was []RequestItem
    Teardown      Section           `yaml:"teardown,omitempty"` // was []RequestItem
    Options       Options           `yaml:"options,omitempty"`
    ExternalFiles []string          `yaml:"-"`
}

type RequestItem struct {
    ...
    Retry *retry.FullConfig `yaml:"retry,omitempty"` // was *RetryConfig
    ...
}
```

**Mechanical changes required:**
- `col.Setup` → `col.Setup.Items` (all slice access)
- `col.Requests` → `col.Requests.Items` (all slice access)
- `col.Teardown` → `col.Teardown.Items` (all slice access)
- `len(col.Setup)` → `len(col.Setup.Items)`
- Test constructions: `Setup: []RequestItem{...}` → `Setup: Section{Items: []RequestItem{...}}`
- Remove `parser.RetryConfig` type (replaced by `retry.FullConfig`)

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_withSectionRetry(t *testing.T) {
    // testdata/with_section_retry.yaml — setup has retry, teardown has retry disabled
    // Verify Section.Retry is parsed for setup and teardown
    // Verify Section.Items are parsed correctly
}

func TestSection_UnmarshalYAML_arrayForm(t *testing.T) {
    // Plain array: setup: [- name: ...] → Items populated, Retry nil
}

func TestSection_UnmarshalYAML_objectForm(t *testing.T) {
    // Object: setup: { retry: {...}, items: [...] } → both populated
}

func TestSection_UnmarshalYAML_emptyMapping(t *testing.T) {
    // Object with only retry, no items → Items nil, Retry set
}
```

**Test data file** `internal/parser/testdata/with_section_retry.yaml`:
```yaml
name: Section Retry Test
retry:
  enabled: true
  max_attempts: 3
setup:
  retry:
    enabled: true
    max_attempts: 5
  items:
    - name: Auth setup
      request:
        method: GET
        url: https://example.com/auth
requests:
  - name: Main request
    request:
      method: GET
      url: https://example.com/api
teardown:
  retry:
    enabled: false
  items:
    - name: Cleanup
      request:
        method: DELETE
        url: https://example.com/cleanup
```

#### Impact on Existing Tests

| Test File | Impact | Action Required |
|-----------|--------|----------------|
| `internal/parser/parser_test.go` | ~72 references to `.Setup`, `.Requests`, `.Teardown` | Update to use `.Items` accessor |
| `internal/parser/external_test.go` | ~31 references | Same mechanical update |
| `internal/parser/testdata/with_retry.yaml` | parser now uses `*retry.FullConfig` | Tests updated to check pointer fields |
| `internal/parser/testdata/with_request_retry.yaml` | same | Same |

---

### Step 3: Add global `defaults.retry` parsing to `internal/config/`
**Rationale:** Global config is needed by the runner to build the full precedence chain. Isolated change to config package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add `Defaults` struct, parse `defaults.retry` from YAML |
| `internal/config/project_test.go` | modify | Add tests for defaults.retry parsing |

#### Current Code
```go
type ProjectConfig struct {
    ProjectName  string
    Variables    map[string]string
    Secrets      *vault.SecretsConfig
    AuthProfiles []auth.Profile
}

type projectFile struct {
    ProjectName  string         `yaml:"project_name"`
    Variables    map[string]any `yaml:"variables"`
    Secrets      yaml.Node      `yaml:"secrets,omitempty"`
    AuthProfiles yaml.Node      `yaml:"auth_profiles,omitempty"`
}
```

#### New Code
```go
type DefaultsConfig struct {
    Retry *retry.FullConfig `yaml:"retry,omitempty"`
}

type ProjectConfig struct {
    ProjectName  string
    Variables    map[string]string
    Secrets      *vault.SecretsConfig
    AuthProfiles []auth.Profile
    Defaults     DefaultsConfig // NEW
}

type projectFile struct {
    ProjectName  string         `yaml:"project_name"`
    Variables    map[string]any `yaml:"variables"`
    Secrets      yaml.Node      `yaml:"secrets,omitempty"`
    AuthProfiles yaml.Node      `yaml:"auth_profiles,omitempty"`
    Defaults     yaml.Node      `yaml:"defaults,omitempty"` // NEW
}

// In ParseProjectConfig, after auth profiles:
if pf.Defaults.Kind != 0 {
    var defaults DefaultsConfig
    if err := pf.Defaults.Decode(&defaults); err != nil {
        return nil, fmt.Errorf("%w: defaults: %s", ErrInvalidProjectConfig, err)
    }
    cfg.Defaults = defaults
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseProjectConfig_Defaults(t *testing.T) {
    tests := []struct {
        name         string
        yaml         string
        wantRetryNil bool
        wantEnabled  *bool
        wantMax      *int
        wantErr      bool
    }{
        {"no defaults block", "project_name: test\n", true, nil, nil, false},
        {"defaults with retry", "defaults:\n  retry:\n    enabled: true\n    max_attempts: 5\n", false, BoolPtr(true), IntPtr(5), false},
        {"defaults with retry disabled", "defaults:\n  retry:\n    enabled: false\n", false, BoolPtr(false), nil, false},
        {"invalid defaults", "defaults: not_a_map\n", true, nil, nil, true},
    }
}
```

#### Impact on Existing Tests
- No existing tests broken — `DefaultsConfig` zero value has nil `Retry` field

---

### Step 4: Wire precedence chain through runner
**Rationale:** Final step — connects all the pieces. Updates runner to merge configs at all levels before executing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `GlobalRetry` to `VarSources`, update `effectiveRetryConfig`, update `runPhases`/`executePhase` signatures |
| `internal/runner/runner_test.go` | modify | Update Collection constructions (Section type), add precedence tests |
| `internal/validator/validator.go` | modify | Update `col.Setup` → `col.Setup.Items` etc. |
| `cmd/apitest/main.go` | modify | Pass `GlobalRetry` from project config to `VarSources` |

#### Current Code (`effectiveRetryConfig`)
```go
func effectiveRetryConfig(collection parser.RetryConfig, request *parser.RetryConfig) parser.RetryConfig {
    if request != nil {
        return *request
    }
    return collection
}

func toRetryConfig(cfg parser.RetryConfig) retry.Config {
    return retry.Config{
        Enabled:        cfg.Enabled,
        MaxAttempts:    cfg.MaxAttempts,
        InitialDelayMs: cfg.InitialDelayMs,
    }
}
```

#### New Code
```go
// VarSources — add field:
type VarSources struct {
    ...
    GlobalRetry *retry.FullConfig // from apitest.yaml defaults.retry
}

// resolveRetryConfig merges all precedence levels and produces a concrete Config.
func resolveRetryConfig(
    global *retry.FullConfig,    // apitest.yaml defaults.retry
    collection *retry.FullConfig, // collection top-level retry:
    section *retry.FullConfig,    // setup/requests/teardown retry:
    request *retry.FullConfig,    // per-request retry:
) retry.Config {
    configs := []retry.FullConfig{retry.BuiltinDefaults()}
    if global != nil {
        configs = append(configs, *global)
    }
    if collection != nil {
        configs = append(configs, *collection)
    }
    if section != nil {
        configs = append(configs, *section)
    }
    if request != nil {
        configs = append(configs, *request)
    }
    return retry.MergeAll(configs...).Resolve()
}

// executePhase signature updated:
func executePhase(
    ...
    collectionRetry *retry.FullConfig,  // was parser.RetryConfig
    sectionRetry *retry.FullConfig,     // NEW
    globalRetry *retry.FullConfig,      // NEW
) ([]RequestResult, bool, error) {
    ...
    // Replace effectiveRetryConfig call with:
    retryCfg := resolveRetryConfig(globalRetry, collectionRetry, sectionRetry, item.Retry)
    ...
}

// runPhases passes section retry from each Section:
func runPhases(...) {
    // Phase 1: Setup
    executePhase(ctx, col.Setup.Items, ..., col.Retry, col.Setup.Retry, vars.GlobalRetry)
    // Phase 2: Main
    executePhase(ctx, col.Requests.Items, ..., col.Retry, col.Requests.Retry, vars.GlobalRetry)
    // Phase 3: Teardown
    executePhase(ctx, col.Teardown.Items, ..., col.Retry, col.Teardown.Retry, vars.GlobalRetry)
}
```

Remove `effectiveRetryConfig` and `toRetryConfig` (replaced by `resolveRetryConfig`).

**cmd/apitest/main.go** — pass global retry:
```go
vars := runner.VarSources{
    ...
    GlobalRetry: projCfg.Defaults.Retry,
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go

func TestResolveRetryConfig_precedence(t *testing.T) {
    tests := []struct {
        name       string
        global     *retry.FullConfig
        collection *retry.FullConfig
        section    *retry.FullConfig
        request    *retry.FullConfig
        want       retry.Config
    }{
        {"all nil uses builtin defaults", nil, nil, nil, nil, retry.Config{Enabled: false, MaxAttempts: 3, InitialDelayMs: 1000}},
        {"global overrides builtin", ...},
        {"collection overrides global", ...},  // Behavior 1
        {"section overrides collection", ...},
        {"request overrides collection", ...}, // Behavior 2
        {"request overrides section", ...},
        {"section setup with max_attempts 5", ...}, // Behavior 3
        {"teardown disabled overrides", ...},        // Behavior 4
    }
}

func TestRun_sectionRetry(t *testing.T) {
    // Setup section with retry max_attempts: 5
    // Verify setup requests retry 5 times on 503
}

func TestRun_teardownRetryDisabled(t *testing.T) {
    // Teardown section with retry enabled: false
    // Verify teardown requests don't retry even with collection retry enabled
}

func TestRun_globalRetryConfig(t *testing.T) {
    // VarSources.GlobalRetry sets enabled: true
    // Collection has no retry block
    // Verify requests retry
}
```

#### Impact on Existing Tests

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `runner_test.go` | All tests constructing `Collection` | `Setup:`, `Requests:`, `Teardown:` field types changed to `Section` | Wrap slices in `Section{Items: ...}` |
| `runner_test.go` | `TestRun_retryOn503` | `Retry` field type changed to `*retry.FullConfig` | Use `&retry.FullConfig{Enabled: retry.BoolPtr(true), ...}` |
| `runner_test.go` | `effectiveRetryConfig` tests (if any) | Function replaced | Remove/update |
| `runner_test.go` | `executePhase` calls | Signature changed | Update to pass section/global retry |
| `validator.go` | References to `col.Setup` etc. | Slice → Section | Use `.Items` |
| `cmd/apitest/main.go` | VarSources construction | New field | Add `GlobalRetry` |

---

## Test Impact Summary

| Test File | Scope of Change | Mechanical? |
|-----------|----------------|-------------|
| `internal/retry/merge_test.go` | **New** — all merge logic tests | N/A |
| `internal/parser/parser_test.go` | ~72 field references + new section tests | Yes (`.Items` suffix) |
| `internal/parser/external_test.go` | ~31 field references | Yes |
| `internal/config/project_test.go` | Add defaults tests, no existing breakage | N/A |
| `internal/runner/runner_test.go` | ~117 field references + new precedence tests + retry field type changes | Mostly mechanical |
| `internal/validator/validator.go` | ~4 references | Yes |
| `cmd/apitest/main.go` | ~4 references + new field | Yes |

## Risks and Edge Cases

- **Risk:** Large blast radius from Section type change (~122 references across 6 files)
  → **Mitigation:** All changes are mechanical (add `.Items`). Do Step 2 in a single focused pass, run `go build ./...` after to catch all sites.

- **Risk:** Custom UnmarshalYAML for Section must handle both array and object YAML forms
  → **Mitigation:** Extensive table-driven tests covering both forms, empty sections, and edge cases.

- **Risk:** Existing tests construct `parser.RetryConfig{}` which gets replaced by `*retry.FullConfig`
  → **Mitigation:** grep for all `RetryConfig` usages and update. The compiler will catch any missed sites.

- **Edge case:** Section YAML is present but empty (`setup:` with no value)
  → **Handling:** Zero-value `Section{}` — `Items` is nil, `Retry` is nil. Same as current behavior.

- **Edge case:** Section YAML is object form but has no `items:` key
  → **Handling:** `Section{Retry: cfg, Items: nil}` — valid, means section has retry policy but no requests.

- **Edge case:** All config levels are nil/omitted
  → **Handling:** `resolveRetryConfig` starts with `BuiltinDefaults()`, so there's always a valid base.

- **Edge case:** `retry_on` deep merge with partially overlapping sub-fields
  → **Handling:** Non-nil array fields (e.g., `status_codes`) in overlay replace base's array. Nil/omitted sub-fields inherit from base.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create apitest.yaml with global retry defaults
# Create collection with collection-level and section-level retry
# Run and verify section/request overrides apply
go test ./internal/retry/... -v -run TestMerge
go test ./internal/runner/... -v -run TestRun_sectionRetry
go test ./internal/runner/... -v -run TestResolveRetryConfig
```
