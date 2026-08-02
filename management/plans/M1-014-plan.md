# Implementation Plan: M1-014

## Overview
Add request-level variable parsing, `--env-var` CLI flag for importing OS environment variables, and implement the full variable precedence chain. This completes the variable system by filling in precedence levels 8 (request) and 9 (env-var).

## Task Details
- **ID:** M1-014
- **Title:** Request-level variables, --env-var, full precedence
- **Phase:** M1: Core CLI
- **Priority:** 14
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-013 | .env file loading for local secrets | done |

## Implementation Steps

### Step 1: Add `Variables` field to `parser.RequestItem`
**Rationale:** Pure data structure change with zero behavior impact. All existing YAML files without `variables:` on requests unmarshal to nil. Smallest possible blast radius.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Variables map[string]string` field to `RequestItem` |
| `internal/parser/parser_test.go` | modify | Add test cases for request-level variables parsing |

#### Current Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Request    Request           `yaml:"request"`
	Variables  map[string]string `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_request_level_variables(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantVars map[string]string
	}{
		{"request with variables", /* yaml with variables: block */, map[string]string{"key": "val"}},
		{"request without variables", /* yaml without variables */, nil},
		{"request with empty variables", /* yaml with variables: {} */, map[string]string{}},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — `Variables` is omitempty and nil by default

---

### Step 2: Add `ParseEnvVarFlag` and sentinel errors to `internal/variable/`
**Rationale:** New standalone function with no changes to existing code. Follows the `ParseVarFlag` pattern already established.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add `ParseEnvVarFlag`, `ErrEnvVarNotSet`, `ErrInvalidEnvVarFlag` |
| `internal/variable/variable_test.go` | modify | Add table-driven tests for `ParseEnvVarFlag` |

#### Current Code
```go
var (
	ErrCircularReference = errors.New("circular variable reference")
	ErrDepthExceeded     = errors.New("variable interpolation depth limit exceeded")
	ErrUndefinedVariable = errors.New("undefined variable")
	ErrInvalidVarFlag    = errors.New("invalid --var flag format")
)
```

#### New Code
```go
var (
	ErrCircularReference = errors.New("circular variable reference")
	ErrDepthExceeded     = errors.New("variable interpolation depth limit exceeded")
	ErrUndefinedVariable = errors.New("undefined variable")
	ErrInvalidVarFlag    = errors.New("invalid --var flag format")
	ErrInvalidEnvVarFlag = errors.New("invalid --env-var flag format")
	ErrEnvVarNotSet      = errors.New("environment variable not set")
)

// ParseEnvVarFlag parses a "--env-var" argument value.
// Supports two forms:
//   "VAR_NAME"          -> imports os.LookupEnv("VAR_NAME") as "VAR_NAME"
//   "VAR_NAME=$OS_VAR"  -> imports os.LookupEnv("OS_VAR") as "VAR_NAME"
// Returns the variable name and its resolved value.
// Returns ErrEnvVarNotSet if the OS environment variable is not set.
// Uses lookupEnv for testability (pass os.LookupEnv in production).
func ParseEnvVarFlag(s string, lookupEnv func(string) (string, bool)) (key, value string, err error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseEnvVarFlag(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		envState  map[string]string // simulated OS env
		wantKey   string
		wantValue string
		wantErr   error
	}{
		{"bare_name_set", "API_KEY", map[string]string{"API_KEY": "secret"}, "API_KEY", "secret", nil},
		{"bare_name_unset", "API_KEY", map[string]string{}, "", "", ErrEnvVarNotSet},
		{"mapped_name_set", "API_KEY=$CI_KEY", map[string]string{"CI_KEY": "secret"}, "API_KEY", "secret", nil},
		{"mapped_name_unset", "API_KEY=$CI_KEY", map[string]string{}, "", "", ErrEnvVarNotSet},
		{"empty_string", "", nil, "", "", ErrInvalidEnvVarFlag},
		{"bare_name_empty_value", "API_KEY", map[string]string{"API_KEY": ""}, "API_KEY", "", nil},
		{"mapped_name_empty_value", "API_KEY=$CI_KEY", map[string]string{"CI_KEY": ""}, "API_KEY", "", nil},
		{"equals_in_env_var_name", "KEY=$VAR_WITH_EQUALS", map[string]string{"VAR_WITH_EQUALS": "val"}, "KEY", "val", nil},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — purely additive

---

### Step 3: Add `WithOverrides` method to `Scope`
**Rationale:** Enables request-level variable scoping without mutating the shared scope. Required before runner changes. Request-level variables must be scoped to a single request — they must not leak to subsequent requests. `WithOverrides` creates a new Scope with overlaid variables.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add `WithOverrides` method to `Scope` |
| `internal/variable/variable_test.go` | modify | Add table-driven tests |

#### New Code
```go
// WithOverrides returns a new Scope that contains all resolved variables
// from the receiver plus the given overrides applied on top. Overrides
// may reference variables from the base scope. The receiver is not mutated.
// Returns nil, nil if overrides is nil or empty (caller should use original scope).
func (s *Scope) WithOverrides(overrides map[string]string) (*Scope, error) {
	if len(overrides) == 0 {
		return s, nil
	}
	// Copy resolved vars from base scope
	merged := make(map[string]string, len(s.resolved)+len(overrides))
	for k, v := range s.resolved {
		merged[k] = v
	}
	// Apply overrides on top
	for k, v := range overrides {
		merged[k] = v
	}
	child := NewScope(merged)
	if err := child.Resolve(); err != nil {
		return nil, err
	}
	return child, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestScope_WithOverrides(t *testing.T) {
	tests := []struct {
		name         string
		baseVars     map[string]string
		overrides    map[string]string
		interpolate  string
		want         string
		wantErr      bool
	}{
		{"adds_new_variable", map[string]string{"a": "1"}, map[string]string{"b": "2"}, "{{b}}", "2", false},
		{"overrides_existing_variable", map[string]string{"a": "old"}, map[string]string{"a": "new"}, "{{a}}", "new", false},
		{"does_not_mutate_original_scope", map[string]string{"a": "original"}, map[string]string{"a": "override"}, "{{a}}", "original", false}, // verify on base scope
		{"nil_overrides_returns_same_scope", map[string]string{"a": "1"}, nil, "{{a}}", "1", false},
		{"empty_overrides_returns_same_scope", map[string]string{"a": "1"}, map[string]string{}, "{{a}}", "1", false},
		{"override_references_base_variable", map[string]string{"host": "example.com"}, map[string]string{"url": "https://{{host}}/api"}, "{{url}}", "https://example.com/api", false},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected — purely additive

---

### Step 4: Introduce `VarSources` struct and refactor `runner.Run` signature
**Rationale:** The current `Run(ctx, col, exec, envVars, dotenvVars, cliVars)` has 6 parameters and needs a 7th (envVarVars). Bundling all variable sources into a struct makes the API cleaner and future-proof. This is a mechanical refactor that touches ~60 call sites in tests but is semantically trivial. Must be done before adding new behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `VarSources` struct, change `Run` signature |
| `internal/runner/runner_test.go` | modify | Update all `Run()` call sites to use `VarSources{}` |
| `cmd/apitest/main.go` | modify | Update `runCmd` to pass `VarSources` |

#### Current Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, envVars, dotenvVars, cliVars map[string]string) ([]RequestResult, *Summary, error) {
```

#### New Code
```go
// VarSources bundles all variable sources for a collection run.
type VarSources struct {
	EnvFile map[string]string // --env file variables (precedence 3)
	DotEnv  map[string]string // .env variables (precedence 4)
	EnvVar  map[string]string // --env-var OS imports (precedence 9)
	CLI     map[string]string // --var variables (precedence 10)
}

func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, vars VarSources) ([]RequestResult, *Summary, error) {
```

All existing test call sites change from:
```go
Run(context.Background(), col, exec, nil, nil, nil)
```
to:
```go
Run(context.Background(), col, exec, VarSources{})
```

And calls with actual values:
```go
Run(context.Background(), col, exec, envVars, dotenvVars, cliVars)
```
become:
```go
Run(context.Background(), col, exec, VarSources{EnvFile: envVars, DotEnv: dotenvVars, CLI: cliVars})
```

#### Tests to Write FIRST (RED phase)

No new tests — this is a pure refactor. Existing tests must continue to pass after updating call sites.

#### Impact on Existing Tests
- **All ~60 call sites in `runner_test.go`** need mechanical update from positional args to struct literal
- **`cmd/apitest/main.go:117`** needs update from `runner.Run(ctx, col, httpexec.Execute, envVars, dotenvVars, cliVars)` to `runner.Run(ctx, col, httpexec.Execute, runner.VarSources{EnvFile: envVars, DotEnv: dotenvVars, CLI: cliVars})`

---

### Step 5: Implement request-level variables and `--env-var` precedence in runner
**Rationale:** Core behavior change — depends on Steps 1-4 being complete. Adds request-level variable scoping (precedence 8) and env-var injection (precedence 9).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add env-var precedence (after resolve, before CLI), use `WithOverrides` for request-level vars |
| `internal/runner/runner_test.go` | modify | Add new test functions for request-level and env-var behavior |

#### Current Code (precedence chain in Run)
```go
// Build merged variable map: env (level 3) < .env (level 4) < collection (level 7)
merged := make(map[string]string, len(envVars)+len(dotenvVars)+len(col.Variables))
for k, v := range envVars {
	merged[k] = v
}
for k, v := range dotenvVars {
	merged[k] = v
}
for k, v := range col.Variables {
	merged[k] = v
}

scope := variable.NewScope(merged)
if err := scope.Resolve(); err != nil {
	return nil, summary, err
}

// CLI --var overrides (precedence 10 — highest)
for k, v := range cliVars {
	scope.Set(k, v)
}
```

#### New Code
```go
// Build merged variable map: env (3) < .env (4) < collection (7)
merged := make(map[string]string, len(vars.EnvFile)+len(vars.DotEnv)+len(col.Variables))
for k, v := range vars.EnvFile {
	merged[k] = v
}
for k, v := range vars.DotEnv {
	merged[k] = v
}
for k, v := range col.Variables {
	merged[k] = v
}

scope := variable.NewScope(merged)
if err := scope.Resolve(); err != nil {
	return nil, summary, err
}

// --env-var imports (precedence 9)
for k, v := range vars.EnvVar {
	scope.Set(k, v)
}

// CLI --var overrides (precedence 10 — highest)
for k, v := range vars.CLI {
	scope.Set(k, v)
}
```

And in the request loop, before interpolation:
```go
// Per-request variable scoping (precedence 8)
reqScope := scope
if len(item.Variables) > 0 {
	var oErr error
	reqScope, oErr = scope.WithOverrides(item.Variables)
	if oErr != nil {
		return nil, summary, fmt.Errorf("request %q variables: %w", item.Name, oErr)
	}
}

interpolated, interpErr := interpolateRequest(reqScope, &req)
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_request_level_variables_override_collection(t *testing.T)
// Collection var base_url="wrong", request var base_url="right"
// Assert request receives correct URL

func TestRun_request_level_variables_scoped_to_single_request(t *testing.T)
// Request 1 has variable override, request 2 does not
// Assert request 2 uses collection-level value

func TestRun_request_level_variables_reference_collection_vars(t *testing.T)
// Collection var host="example.com", request var url="https://{{host}}/api"
// Assert interpolation chain works

func TestRun_env_var_overrides_collection(t *testing.T)
// Collection var key="col", VarSources.EnvVar key="env"
// Assert env-var wins (precedence 9 > 7)

func TestRun_env_var_overrides_request_level(t *testing.T)
// Request var key="req", VarSources.EnvVar key="env"
// Assert env-var wins (precedence 9 > 8)

func TestRun_cli_var_overrides_env_var(t *testing.T)
// VarSources.EnvVar key="env", VarSources.CLI key="cli"
// Assert CLI wins (precedence 10 > 9)

func TestRun_full_precedence_chain(t *testing.T)
// All levels set same var, assert highest-precedence wins

func TestRun_request_level_vars_with_extraction(t *testing.T)
// Request 1 extracts a var, request 2 uses it
// Request 2 also has local vars — assert both are available

func TestRun_request_level_vars_do_not_leak_to_next_request(t *testing.T)
// Request 1 sets local var "x", request 2 doesn't define "x"
// Assert request 2 does not see "x" (unless defined at collection level)
```

#### Impact on Existing Tests
- No existing tests break — new behavior is additive (request-level vars default to nil, env-var defaults to nil in VarSources{})

---

### Step 6: Add `--env-var` flag parsing to CLI
**Rationale:** CLI plumbing to surface the new env-var capability. Depends on `ParseEnvVarFlag` from Step 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `--env-var` to `parseRunArgs`, wire through `runCmd`, update `printHelp` |
| `cmd/apitest/main_test.go` | modify | Add `--env-var` test cases to `TestParseRunArgs`, add integration tests |

#### Current Code
```go
func parseRunArgs(args []string) (file, envName string, vars map[string]string, err error) {
```

#### New Code
```go
func parseRunArgs(args []string) (file, envName string, vars, envVarVars map[string]string, err error) {
```

The `--env-var` flag handling in the switch:
```go
case "--env-var":
	i++
	if i >= len(args) {
		return "", "", nil, nil, fmt.Errorf("--env-var requires a value (e.g. --env-var API_KEY)")
	}
	k, v, parseErr := variable.ParseEnvVarFlag(args[i], os.LookupEnv)
	if parseErr != nil {
		return "", "", nil, nil, parseErr
	}
	envVarVars[k] = v
```

And in `runCmd`, wire it:
```go
file, envName, cliVars, envVarVars, parseErr := parseRunArgs(args)
// ...
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
	EnvFile: envVars,
	DotEnv:  dotenvVars,
	EnvVar:  envVarVars,
	CLI:     cliVars,
})
```

Help text addition:
```
  --env-var VAR_NAME      Import OS environment variable (repeatable)
  --env-var VAR=$OS_VAR   Import and rename OS environment variable
```

#### Tests to Write FIRST (RED phase)

```go
// In TestParseRunArgs table:
{"env_var_bare", []string{"col.yaml", "--env-var", "API_KEY"}, "col.yaml", "", map[string]string{}, map[string]string{"API_KEY": "secret"}, false},
{"env_var_mapped", []string{"col.yaml", "--env-var", "KEY=$OS_KEY"}, "col.yaml", "", map[string]string{}, map[string]string{"KEY": "secret"}, false},
{"env_var_multiple", []string{"col.yaml", "--env-var", "A", "--env-var", "B"}, ...},
{"env_var_without_value", []string{"col.yaml", "--env-var"}, "", "", nil, nil, true},
{"env_var_with_other_flags", []string{"col.yaml", "--env", "dev", "--env-var", "X", "--var", "y=1"}, ...},

// Integration tests:
func TestCLIIntegration_env_var_imports_os_env(t *testing.T)
func TestCLIIntegration_env_var_missing_os_env_errors(t *testing.T)
func TestCLIIntegration_env_var_mapped_name(t *testing.T)
func TestCLIIntegration_env_var_precedence_over_collection(t *testing.T)
func TestCLIIntegration_cli_var_precedence_over_env_var(t *testing.T)
func TestCLIIntegration_full_precedence_chain(t *testing.T)
```

#### Impact on Existing Tests
- **All `TestParseRunArgs` cases** need updating for new return value (`envVarVars`)
- **`runCmd` callers** in integration tests are unaffected (they test the binary, not the function)

---

### Step 7: Update smoke test
**Rationale:** Completeness contract requires smoke test coverage for new user-facing capabilities.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add `--env-var` smoke test |

#### Tests to Add
- Test `--env-var` with a real OS env var set
- Test `--env-var` help text appears in `--help` output

#### Impact on Existing Tests
- No existing smoke tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | (new tests) | new | Add request-level variable parsing tests |
| `internal/variable/variable_test.go` | (new tests) | new | Add `ParseEnvVarFlag` and `WithOverrides` tests |
| `internal/runner/runner_test.go` | All ~60 `Run()` calls | signature change | Update from positional args to `VarSources{}` |
| `internal/runner/runner_test.go` | (new tests) | new | Add request-level and env-var precedence tests |
| `cmd/apitest/main_test.go` | `TestParseRunArgs` | return value change | Add `envVarVars` to all test cases |
| `cmd/apitest/main_test.go` | (new tests) | new | Add `--env-var` integration tests |
| `smoke/run.sh` | (new test) | new | Add `--env-var` smoke test |

## Risks and Edge Cases

- **Risk:** `--env-var API_KEY` when `API_KEY=""` (set but empty) → **Mitigation:** Use `os.LookupEnv` not `os.Getenv`. Empty-but-set is valid. Only error when truly unset.
- **Risk:** Request-level variable references undefined variable → **Mitigation:** `Scope.Resolve()` inside `WithOverrides` catches this, producing a clear error with available variables listed.
- **Risk:** Request-level variables leaking to subsequent requests → **Mitigation:** `WithOverrides` creates a new scope; the base scope is never mutated. Extracted variables are set on the base scope and remain available to all subsequent requests.
- **Risk:** `--env-var` and `--var` both set same key → **Mitigation:** `--var` (10) wins over `--env-var` (9) by design — `scope.Set` for CLI vars happens after env-var injection.
- **Edge case:** Empty `--env-var` argument → **Handling:** Return error with usage hint.
- **Edge case:** Request-level vars create circular reference with collection vars → **Handling:** `WithOverrides` calls `Resolve()` which detects cycles.
- **Edge case:** Extracted variable (from prior request) vs request-level variable (current request) → **Handling:** Both are in the merged scope when `WithOverrides` is called. Request-level overrides extracted values within that request only, which is correct (precedence 8 > 7).
- **Risk:** ~60 test call sites changing → **Mitigation:** Mechanical find-replace, verified by `go test ./...` passing.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Set OS env var and verify --env-var imports it
export TEST_API_KEY=my-secret
apitest run test.yaml --env-var TEST_API_KEY

# Verify mapped env var
export CI_API_KEY=ci-secret
apitest run test.yaml --env-var API_KEY=$CI_API_KEY

# Verify full precedence chain
# Create collection with variables, env file, .env, request vars
# Run with --env, --env-var, and --var flags
# Assert highest-precedence value wins at each level
```
