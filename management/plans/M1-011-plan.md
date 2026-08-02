# Implementation Plan: M1-011

## Overview
Add `--var key=value` CLI flag to the `run` command, allowing users to override collection-level variables at the highest precedence level (10).

## Task Details
- **ID:** M1-011
- **Title:** CLI --var flag for variable overrides
- **Phase:** M1: Core CLI
- **Priority:** 11
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-009 | Collection-level variables with interpolation | done |

## Implementation Steps

### Step 1: Add `ParseVarFlag` function in `internal/variable/`
**Rationale:** Pure function with zero dependencies on existing code — fully testable in isolation, smallest blast radius.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Add `ParseVarFlag(s string) (key, value string, err error)` |
| `internal/variable/variable_test.go` | modify | Add table-driven tests for flag parsing |

#### New Code
```go
// ErrInvalidVarFlag is returned when a --var flag value cannot be parsed.
var ErrInvalidVarFlag = errors.New("invalid --var flag format")

// ParseVarFlag parses a "--var" argument value of the form "key=value".
// Splits on first "="; key must be non-empty. Value may be empty.
func ParseVarFlag(s string) (key, value string, err error) {
    idx := strings.Index(s, "=")
    if idx < 0 {
        return "", "", fmt.Errorf("%w: expected key=value, got %q", ErrInvalidVarFlag, s)
    }
    key = s[:idx]
    if key == "" {
        return "", "", fmt.Errorf("%w: empty key in %q", ErrInvalidVarFlag, s)
    }
    return key, s[idx+1:], nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseVarFlag(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantKey   string
        wantValue string
        wantErr   bool
    }{
        {"simple_key_value", "base_url=http://localhost:8080", "base_url", "http://localhost:8080", false},
        {"value_with_equals", "name=value=with=equals", "name", "value=with=equals", false},
        {"empty_value", "key=", "key", "", false},
        {"url_value", "url=https://example.com/api?q=1&x=2", "url", "https://example.com/api?q=1&x=2", false},
        {"no_equals_sign", "justkey", "", "", true},
        {"empty_key", "=value", "", "", true},
        {"empty_string", "", "", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            key, value, err := ParseVarFlag(tt.input)
            if tt.wantErr {
                if err == nil {
                    t.Fatal("expected error, got nil")
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if key != tt.wantKey {
                t.Errorf("key = %q, want %q", key, tt.wantKey)
            }
            if value != tt.wantValue {
                t.Errorf("value = %q, want %q", value, tt.wantValue)
            }
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Add `--var` extraction to CLI arg parsing
**Rationale:** Builds on Step 1. Changes CLI layer only — runner not yet touched.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `parseRunArgs` function and update `runCmd` |
| `cmd/apitest/main_test.go` | modify | Add unit tests for arg parsing |

#### Current Code
```go
func runCmd(args []string) int {
    if len(args) == 0 {
        // ...
    }
    col, err := parser.ParseFile(args[0])
    // ...
    results, summary, varErr := runner.Run(ctx, col, httpexec.Execute)
```

#### New Code
```go
// parseRunArgs extracts the collection file path and --var overrides from args.
func parseRunArgs(args []string) (file string, vars map[string]string, err error) {
    vars = make(map[string]string)
    var positional []string
    for i := 0; i < len(args); i++ {
        if args[i] == "--var" {
            i++
            if i >= len(args) {
                return "", nil, fmt.Errorf("--var requires a value (e.g. --var key=value)")
            }
            k, v, err := variable.ParseVarFlag(args[i])
            if err != nil {
                return "", nil, err
            }
            vars[k] = v
        } else {
            positional = append(positional, args[i])
        }
    }
    if len(positional) == 0 {
        return "", nil, fmt.Errorf("missing collection file path")
    }
    return positional[0], vars, nil
}

func runCmd(args []string) int {
    file, cliVars, err := parseRunArgs(args)
    if err != nil {
        // ... structured error
    }
    col, err := parser.ParseFile(file)
    // ...
    results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, cliVars)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantFile string
        wantVars map[string]string
        wantErr  bool
    }{
        {"file_only", []string{"col.yaml"}, "col.yaml", map[string]string{}, false},
        {"file_with_one_var", []string{"col.yaml", "--var", "k=v"}, "col.yaml", map[string]string{"k": "v"}, false},
        {"file_with_multiple_vars", []string{"col.yaml", "--var", "a=1", "--var", "b=2"}, "col.yaml", map[string]string{"a": "1", "b": "2"}, false},
        {"var_before_file", []string{"--var", "k=v", "col.yaml"}, "col.yaml", map[string]string{"k": "v"}, false},
        {"no_file", []string{"--var", "k=v"}, "", nil, true},
        {"var_without_value", []string{"col.yaml", "--var"}, "", nil, true},
        {"var_invalid_format", []string{"col.yaml", "--var", "noequals"}, "", nil, true},
        {"no_args", []string{}, "", nil, true},
    }
    // ...
}
```

#### Impact on Existing Tests
- Existing `runCmd` integration tests pass `nil` cliVars implicitly (no `--var` flags in args) — should be unaffected since `parseRunArgs` handles args without `--var` flags correctly.

---

### Step 3: Update `runner.Run` to accept CLI variables
**Rationale:** Depends on Steps 1-2. This is the core integration — CLI vars override collection vars via `scope.Set()`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `cliVars map[string]string` parameter to `Run`, apply after `scope.Resolve()` |
| `internal/runner/runner_test.go` | modify | Add override tests + update all existing `Run()` calls with `nil` |

#### Current Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc) ([]RequestResult, *Summary, error) {
    // ...
    scope := variable.NewScope(col.Variables)
    if err := scope.Resolve(); err != nil {
        return nil, nil, fmt.Errorf("resolving variables: %w", err)
    }
```

#### New Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, cliVars map[string]string) ([]RequestResult, *Summary, error) {
    // ...
    scope := variable.NewScope(col.Variables)
    if err := scope.Resolve(); err != nil {
        return nil, nil, fmt.Errorf("resolving variables: %w", err)
    }
    // CLI --var overrides (precedence 10 — highest)
    for k, v := range cliVars {
        scope.Set(k, v)
    }
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_cli_var_overrides_collection_variable(t *testing.T) {
    // Collection has base_url=old, CLI has base_url=new
    // Verify interpolated URL uses "new"
}

func TestRun_cli_var_multiple(t *testing.T) {
    // Two CLI vars, both used in request URLs
}

func TestRun_cli_var_nil_unchanged(t *testing.T) {
    // Pass nil for cliVars, existing behavior unchanged
}

func TestRun_cli_var_adds_new_variable(t *testing.T) {
    // CLI var not in collection, used in request URL
}
```

#### Impact on Existing Tests
- **All existing `runner.Run()` calls** need `nil` appended as the 4th argument (~25+ call sites in `runner_test.go`)
- This is mechanical — no behavior change when `cliVars` is `nil`

---

### Step 4: Update help text
**Rationale:** User-facing change — update docs to reflect new capability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update `printHelp()` to show `--var` flag |

#### Current Code
```go
// Current help text (read actual content from printHelp)
```

#### New Code
```
Usage:
  apitest <command> [arguments]

Commands:
  run <file>   Execute requests in a collection file

Run Options:
  --var key=value   Set a variable (overrides collection variables, repeatable)

Options:
  --version    Show version information
  --help       Show this help message
```

#### Impact on Existing Tests
- Help text integration tests may need updating if they assert exact output

---

### Step 5: CLI integration tests
**Rationale:** End-to-end validation after all pieces are in place.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add integration tests with test HTTP server |
| `smoke/run.sh` | modify | Add `--var` override scenario |

#### Tests to Write

```go
func TestCLIIntegration_var_flag_overrides_collection(t *testing.T) {
    // Collection with base_url var, --var base_url=<server.URL>, verify success
}

func TestCLIIntegration_var_flag_multiple(t *testing.T) {
    // Multiple --var flags, verify all applied
}

func TestCLIIntegration_var_flag_invalid_format(t *testing.T) {
    // --var noequals, verify error message and exit code 2
}

func TestCLIIntegration_var_flag_no_value_after(t *testing.T) {
    // --var as last arg, verify error
}

func TestCLIIntegration_var_flag_value_with_equals(t *testing.T) {
    // --var k=v=w, verify value is "v=w"
}
```

#### Impact on Existing Tests
- No existing tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All `Run()` calls | signature change | Append `nil` as 4th arg (~25+ sites) |
| `cmd/apitest/main_test.go` | Help text tests | possible string change | Update expected help output |
| All other test files | — | none | — |

## Risks and Edge Cases

- **Risk:** Changing `runner.Run` signature breaks all existing test call sites → **Mitigation:** Mechanical change — add `nil` as last arg in a single commit to keep tests green
- **Risk:** Arg parsing ambiguity (`--var` as filename) → **Mitigation:** Standard CLI convention — `--var` always consumes next arg. File named `--var` requires `./--var`
- **Edge case:** Duplicate `--var` keys (`--var k=1 --var k=2`) → **Handling:** Last one wins (standard map insertion). Document this behavior
- **Edge case:** CLI vars not participating in chain resolution → **Handling:** By design — CLI vars are literal values (precedence 10), injected via `Scope.Set` after resolution. Correct behavior per spec
- **Risk:** Future flags (`--env-var` in M1-014) will need same pattern → **Mitigation:** `parseRunArgs` is extensible. Refactoring to options struct can happen when needed

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
./apitest run collection.yaml --var base_url=http://localhost:8080
```
