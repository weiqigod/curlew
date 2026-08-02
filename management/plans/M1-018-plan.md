# Implementation Plan: M1-018

## Overview
Adds dynamic variable functions (e.g. `{{$uuid}}`, `{{$timestamp}}`) to the variable resolution system, with an optional `--seed` flag for deterministic/reproducible output.

## Task Details
- **ID:** M1-018
- **Title:** Dynamic variable functions with seed reproducibility
- **Phase:** M1: Core CLI
- **Priority:** 18
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-014 | Request-level variables, --env-var, full precedence | done |

---

## Key Architectural Decisions

1. **New regex for `$`-prefixed patterns:** Current `varPattern` only matches `[a-zA-Z_]` as first char — it will never match `{{$uuid}}`. A second regex `dynPattern = regexp.MustCompile(\{\{\$([a-zA-Z][a-zA-Z0-9_]*)\}\})` is added.

2. **Registry lives in `internal/variable/dynamic.go`:** Pure addition to the variable package. `DynFunc` type + `Registry` struct with seeded `math/rand/v2` RNG.

3. **`--seed` uses `*int64` pointer:** Distinguishes no-seed (nil) from seed=0.

4. **Timestamps are never seeded:** `$timestamp`, `$isoTimestamp`, `$timestampMs` always use real time per spec.

5. **Per-request memoization:** `{{$uuid}}` appearing twice in one request returns the same UUID. A fresh `map[string]string` cache is created per request via `BeginRequest()`/`EndRequest()` on the `Scope`.

6. **Precedence level 1:** Dynamic functions are the lowest priority. Any explicit variable (from any higher-level source, including `--var '$uuid=fixed'`) overrides a dynamic function result — implemented by checking `resolved` map before evaluating the function.

7. **UUID generation without external deps:** Use `crypto/rand` when unseeded, construct UUID v4 from seeded `math/rand/v2` bytes with correct version/variant bits when seeded.

8. **15 functions:** Timestamps (3) + UUID/GUID (2) + random primitives (5) + realistic values (5).

---

## Implementation Steps

### Step 1: Create `internal/variable/dynamic.go` (pure addition)
**Rationale:** Smallest blast radius — entirely new file, nothing existing is changed.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/dynamic.go` | create | Registry type, DynFunc type, all 15 implementations |
| `internal/variable/dynamic_test.go` | create | Comprehensive tests for all functions and seeding |

#### New Code

```go
package variable

import (
    "crypto/rand"
    "fmt"
    "math/rand/v2"
    "sort"
    "strconv"
    "strings"
    "time"
)

// DynFunc generates a dynamic variable value.
// rng is nil when no seed is provided — use crypto/rand or time.Now() as appropriate.
type DynFunc func(rng *rand.Rand) string

// Registry holds registered dynamic variable functions.
type Registry struct {
    funcs map[string]DynFunc
    rng   *rand.Rand // nil = use real randomness
}

// NewRegistry creates a dynamic function registry.
// If seed is non-nil, all RNG-based functions use a deterministic source seeded with *seed.
func NewRegistry(seed *int64) *Registry

// Evaluate returns the value for the named function.
// cache provides per-request memoization: if name already in cache, returns cached value.
// Returns a descriptive error if the function name is not recognized.
func (r *Registry) Evaluate(name string, cache map[string]string) (string, error)

// Available returns a sorted list of all registered function names (without $ prefix).
func (r *Registry) Available() []string
```

#### Tests to Write FIRST (RED phase)

```go
func TestRegistry_New_no_seed(t *testing.T) { ... }
func TestRegistry_New_with_seed(t *testing.T) { ... }

func TestRegistry_Evaluate(t *testing.T) {
    tests := []struct {
        name     string
        funcName string
        validate func(t *testing.T, val string)
    }{
        {"timestamp returns unix seconds",          "$timestamp",      isValidUnixSeconds},
        {"isoTimestamp returns RFC3339",             "$isoTimestamp",   isValidRFC3339},
        {"timestampMs returns milliseconds",         "$timestampMs",    isValidUnixMs},
        {"uuid returns valid v4",                    "$uuid",           isValidUUIDv4},
        {"guid same format as uuid",                 "$guid",           isValidUUIDv4},
        {"randomInt in range 0-1000",                "$randomInt",      isIntInRange(0, 1000)},
        {"randomFloat in range 0-1000",              "$randomFloat",    isFloatInRange(0, 1000)},
        {"randomBoolean is true or false",           "$randomBoolean",  isBoolean},
        {"randomString is 16 alphanumeric chars",    "$randomString",   isAlphaNumLen(16)},
        {"randomHex is 32 hex chars",                "$randomHex",      isHexLen(32)},
        {"randomEmail has @ and example.com",        "$randomEmail",    isEmail},
        {"randomName has two space-separated parts", "$randomName",     isTwoParts},
        {"randomFirstName is non-empty string",      "$randomFirstName", isNonEmpty},
        {"randomLastName is non-empty string",       "$randomLastName",  isNonEmpty},
        {"randomColor matches #rrggbb",              "$randomColor",    isHexColor},
        {"unknown function returns error",           "$unknownFunc",    nil}, // expects error
    }
}

func TestRegistry_seeded_deterministic(t *testing.T) { ... }
func TestRegistry_seeded_uuid_deterministic(t *testing.T) { ... }
func TestRegistry_different_seeds_different_values(t *testing.T) { ... }
func TestRegistry_evaluate_caches_within_request(t *testing.T) { ... }
func TestRegistry_evaluate_different_cache_different_result(t *testing.T) { ... }
func TestRegistry_unknown_function_lists_available(t *testing.T) { ... }
func TestRegistry_available_sorted(t *testing.T) { ... }
func TestRegistry_seed_zero_is_valid(t *testing.T) { ... }
```

#### Impact on Existing Tests
- No existing tests affected — this is a brand new file.

---

### Step 2: Extend `internal/variable/variable.go` with dynamic function support
**Rationale:** Builds on Step 1; modifies the variable interpolation to handle `{{$func}}` patterns.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add dynPattern, WithDynamic, BeginRequest/EndRequest, update Interpolate |
| `internal/variable/variable_test.go` | modify | Add dynamic interpolation test cases |

#### Current Code

```go
// internal/variable/variable.go (relevant excerpt)
var varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)

type Scope struct {
    vars     map[string]string
    resolved map[string]string
}
```

#### New Code

```go
var varPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_]*)\}\}`)
var dynPattern = regexp.MustCompile(`\{\{\$([a-zA-Z][a-zA-Z0-9_]*)\}\}`)

type Scope struct {
    vars      map[string]string
    resolved  map[string]string
    registry  *Registry         // nil = no dynamic functions
    funcCache map[string]string // per-request memoization; nil outside a request
}

// WithDynamic returns a shallow copy of the scope with the given registry attached.
func (s *Scope) WithDynamic(r *Registry) *Scope

// BeginRequest initialises a fresh per-request function value cache.
func (s *Scope) BeginRequest()

// EndRequest clears the per-request function cache.
func (s *Scope) EndRequest()

// Interpolate resolves dynamic functions (level 1) then regular variables.
// Dynamic functions left unresolved when no registry is set.
func (s *Scope) Interpolate(input string) (string, error)
```

`Interpolate` updated logic:
1. Find all `{{$funcName}}` matches using `dynPattern` in order (left-to-right)
2. For each: if `$funcName` exists in `resolved` map, use that value (explicit override wins)
3. Otherwise, if registry is non-nil, call `registry.Evaluate(funcName, s.funcCache)`
4. Replace matches; emit error for unknown functions
5. Continue with existing `varPattern` resolution

#### Tests to Write FIRST (RED phase)

```go
func TestScope_Interpolate_dynamic(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        vars    map[string]string  // explicit variables
        // ...
    }{
        {"uuid in url",                      "https://api/{{$uuid}}",          nil},
        {"timestamp replaced",               "ts={{$timestamp}}",               nil},
        {"two references same value",        "{{$uuid}}-{{$uuid}}",             nil}, // same UUID both sides
        {"explicit var overrides function",  "{{$uuid}}",   map[string]string{"$uuid": "fixed"}},
        {"mixed normal and dynamic vars",    "{{base}}/{{$uuid}}",              map[string]string{"base": "http://x"}},
        {"unknown function returns error",   "{{$unknownFunc}}",                nil},
        {"no registry leaves pattern",       "{{$uuid}}",                       nil}, // without WithDynamic
    }
}
```

#### Impact on Existing Tests
- `Scope` struct gains two new zero-value-safe fields — no existing construction code changes.
- `Interpolate` now processes a second pass; if no `$`-prefixed patterns exist in the test input, behavior is identical.
- No existing test cases are expected to break.

---

### Step 3: Add `--seed` flag parsing in `cmd/apitest/main.go`
**Rationale:** CLI-only change; doesn't touch runner until Step 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add --seed to parseRunArgs, update runCmd, update help text |
| `cmd/apitest/main_test.go` | modify | Add seed parsing test cases |

#### Current Code

```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, err error)
```

#### New Code

```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, seed *int64, err error)
```

Parsing logic addition inside the switch:
```go
case "--seed":
    if i+1 >= len(args) {
        return "", "", nil, nil, nil, fmt.Errorf("--seed requires a value")
    }
    i++
    n, parseErr := strconv.ParseInt(args[i], 10, 64)
    if parseErr != nil {
        return "", "", nil, nil, nil, fmt.Errorf("--seed value must be an integer: %w", parseErr)
    }
    seed = &n
```

Help text addition:
```
  --seed <number>       seed for deterministic random variable functions
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs_seed(t *testing.T) {
    tests := []struct {
        name      string
        args      []string
        wantSeed  *int64
        wantErr   bool
    }{
        {"seed 42",              []string{"file.yaml", "--seed", "42"},  ptr(int64(42)),  false},
        {"seed zero",            []string{"file.yaml", "--seed", "0"},   ptr(int64(0)),   false},
        {"seed negative",        []string{"file.yaml", "--seed", "-1"},  ptr(int64(-1)),  false},
        {"seed no value",        []string{"file.yaml", "--seed"},         nil,             true},
        {"seed non-integer",     []string{"file.yaml", "--seed", "abc"}, nil,             true},
        {"no seed flag",         []string{"file.yaml"},                   nil,             false},
    }
}
```

#### Impact on Existing Tests
- `TestParseRunArgs` must capture the new `seed` return value — update all existing table entries to include `_, ` for the seed.
- `runCmd` caller must be updated to pass seed along.

---

### Step 4: Wire `Registry` through `internal/runner/runner.go`
**Rationale:** After CLI changes stabilise, wire the seed into the runner and scope.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add Seed to VarSources, create registry, wire into scope, add BeginRequest/EndRequest |
| `internal/runner/runner_test.go` | modify | Add seed/dynamic-function integration tests |

#### Current Code

```go
type VarSources struct {
    Project map[string]string
    EnvFile map[string]string
    DotEnv  map[string]string
    EnvVar  map[string]string
    CLI     map[string]string
}
```

#### New Code

```go
type VarSources struct {
    Project map[string]string
    EnvFile map[string]string
    DotEnv  map[string]string
    EnvVar  map[string]string
    CLI     map[string]string
    Seed    *int64  // nil = no seed (real randomness)
}
```

In `Run()`, after scope construction:
```go
registry := variable.NewRegistry(vars.Seed)
scope = scope.WithDynamic(registry)
```

In `interpolateRequest()` (within runner):
```go
func interpolateRequest(scope *variable.Scope, req *parser.Request) (*parser.Request, error) {
    scope.BeginRequest()
    defer scope.EndRequest()
    // ... existing interpolation calls unchanged ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_dynamic_uuid_interpolated(t *testing.T) { ... }
func TestRun_seed_deterministic_across_runs(t *testing.T) { ... }
func TestRun_dynamic_different_per_request(t *testing.T) { ... }
func TestRun_dynamic_override_by_cli_var(t *testing.T) { ... }
```

#### Impact on Existing Tests
- `VarSources{}` struct literals without `Seed` field default to nil — no existing test breakage.
- `interpolateRequest` gains `BeginRequest`/`EndRequest` calls which are no-ops when no registry is set.

---

### Step 5: Integration test in `cmd/apitest/main_test.go`
**Rationale:** End-to-end verification of seed determinism using the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add binary-level integration tests for dynamic functions and --seed |

#### Tests to Write FIRST (RED phase)

```go
func TestIntegration_dynamic_uuid_in_request(t *testing.T) {
    // write collection with {{$uuid}} in URL; verify request reaches server
}

func TestIntegration_seed_deterministic(t *testing.T) {
    // run binary twice with --seed 42; verify captured request payloads are identical
}

func TestIntegration_no_seed_produces_random(t *testing.T) {
    // run twice without --seed; verify UUIDs differ
}
```

---

### Step 6: Update smoke test
**Rationale:** Ensures the observable from the task YAML is actually testable end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add cases for {{$uuid}} and --seed determinism |

---

## The 15 Core Dynamic Functions

| # | Name | Seeded? | Output Description |
|---|------|---------|-------------------|
| 1 | `$timestamp` | No | Unix timestamp in seconds (`"1730217600"`) |
| 2 | `$isoTimestamp` | No | RFC 3339 UTC (`"2024-10-29T15:30:00Z"`) |
| 3 | `$timestampMs` | No | Unix timestamp in milliseconds |
| 4 | `$uuid` | Yes | UUID v4 (`"550e8400-e29b-41d4-a716-446655440000"`) |
| 5 | `$guid` | Yes | Alias for `$uuid` (Postman compatibility) |
| 6 | `$randomInt` | Yes | Integer 0–1000 |
| 7 | `$randomFloat` | Yes | Float 0.00–1000.00 (2 decimal places) |
| 8 | `$randomBoolean` | Yes | `"true"` or `"false"` |
| 9 | `$randomString` | Yes | 16-char alphanumeric |
| 10 | `$randomHex` | Yes | 32-char lowercase hex |
| 11 | `$randomEmail` | Yes | `"sarah.4821@example.com"` format |
| 12 | `$randomName` | Yes | `"Sarah Johnson"` format |
| 13 | `$randomFirstName` | Yes | First name from built-in pool (~50 names) |
| 14 | `$randomLastName` | Yes | Last name from built-in pool (~50 names) |
| 15 | `$randomColor` | Yes | `"#a3f2b1"` hex color |

Note: Timestamp functions (`$timestamp`, `$isoTimestamp`, `$timestampMs`) always use `time.Now()` and are not affected by `--seed`.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/apitest/main_test.go` | `TestParseRunArgs` | breaks | Add `seed *int64` to all table entries |
| `cmd/apitest/main_test.go` | all callers of `parseRunArgs` | breaks | Capture new return value |
| `internal/variable/variable_test.go` | all | none | No changes needed |
| `internal/runner/runner_test.go` | all | none | `VarSources{}` zero value still valid |

---

## Risks and Edge Cases

- **Risk:** `$timestamp` is not deterministic even with `--seed`. → **Mitigation:** Document in help text. `--seed` only affects random functions, not timestamp functions.
- **Risk:** Ordering of RNG draws affects determinism if template has mixed functions. → **Mitigation:** Always process matches left-to-right using `FindAllStringSubmatchIndex`.
- **Risk:** `$uuid` and `$guid` may be expected to return the same value. → **Handling:** They are separate functions with separate cache keys; `$guid` draws from the same RNG sequentially after `$uuid`.
- **Risk:** `{{$uuid}}` in a collection-level `variables:` block. → **Handling:** `Resolve()` uses `varPattern` which skips `$`-prefixed names, so `{{$uuid}}` in a variable value passes through untouched and gets expanded at interpolation time. Correct behavior.
- **Edge case:** `--seed` with value `0` must be treated as a valid seed, not as "no seed". → **Handling:** `*int64` pointer: nil = no seed, `&int64(0)` = seed 0.
- **Edge case:** Dynamic function in URL path that contains special characters in the result (e.g., `$randomEmail` generates `user@foo.com` in a URL). → **Handling:** This is the caller's responsibility; the tool interpolates as-is. Document that functions returning special chars may need URL encoding.
- **Risk:** No external dependency for UUID. → **Mitigation:** Hand-roll UUID v4 construction: 16 random bytes, set byte[6] version bits (`0x40`), set byte[8] variant bits (`0x80`), format with dashes.

---

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a collection with dynamic variables
cat > /tmp/dynamic-test.yaml << 'EOF'
requests:
  - name: "test dynamic vars"
    method: GET
    url: "http://httpbin.org/get?id={{$uuid}}&ts={{$timestamp}}"
EOF

# Run twice with same seed — output must be identical
./apitest run /tmp/dynamic-test.yaml --seed 42 > /tmp/run1.txt
./apitest run /tmp/dynamic-test.yaml --seed 42 > /tmp/run2.txt
diff /tmp/run1.txt /tmp/run2.txt  # should produce no output (identical)

# Run without seed — UUIDs should differ
./apitest run /tmp/dynamic-test.yaml 2>&1 | grep -o 'id=[^ &]*'
./apitest run /tmp/dynamic-test.yaml 2>&1 | grep -o 'id=[^ &]*'
```
