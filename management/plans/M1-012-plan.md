# Implementation Plan: M1-012

## Overview
Add `--env` flag to the CLI that loads environment-specific YAML files from an `environments/` directory, with proper variable precedence (environment < collection < CLI `--var`).

## Task Details
- **ID:** M1-012
- **Title:** Environment files with --env flag
- **Phase:** M1: Core CLI
- **Priority:** 12
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-011 | CLI --var flag for variable overrides | done |

## Architecture Notes

### Variable Precedence (from spec)
Per the specification, the precedence levels are:
- Level 3: Environment variables (`environments/dev.yaml`) — **lower**
- Level 7: Collection-level variables — **higher**
- Level 10: CLI `--var` overrides — **highest**

Environment variables serve as defaults that collections can override. CLI `--var` overrides everything.

### Environment File Format
```yaml
# environments/dev.yaml
variables:
  base_url: "http://localhost:8080"
  api_key: "dev-key-123"
  db:
    host: "localhost"
    port: "5432"
```
Nested maps are flattened with underscore separators: `db_host`, `db_port`. This matches the existing variable name regex `[a-zA-Z_][a-zA-Z0-9_]*`.

### File Discovery
1. Search `environments/` directory relative to the collection file location
2. Accept both `.yaml` and `.yml` extensions; prefer `.yaml` when both exist

### Data Flow
```
main.go: parseRunArgs() -> file, envName, cliVars
  -> config.LoadEnvironment(envName, baseDir) -> envVars map[string]string
  -> runner.Run(ctx, col, exec, envVars, cliVars)
    -> merge: envVars (base) + col.Variables (overlay) + cliVars (top)
    -> variable.NewScope(merged) -> Resolve() -> Set() for cliVars
```

## Implementation Steps

### Step 1: Environment file parsing in `internal/config/`
**Rationale:** Smallest blast radius — new package, no existing code affected.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/environment.go` | create | Environment file types, parsing, discovery, listing |
| `internal/config/environment_test.go` | create | Full test coverage for parsing and discovery |
| `internal/config/.gitkeep` | delete | No longer needed with real files |

#### New Code

```go
package config

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"

    "gopkg.in/yaml.v3"
)

var (
    ErrEnvironmentNotFound = errors.New("environment not found")
    ErrInvalidEnvironment  = errors.New("invalid environment file")
)

// ParseEnvironmentFile reads and parses an environment YAML file,
// returning a flat map of variable names to string values.
// Nested maps are flattened using underscore-separated keys.
func ParseEnvironmentFile(path string) (map[string]string, error)

// LoadEnvironment finds and parses the named environment file,
// searching in the environments/ subdirectory of baseDir.
func LoadEnvironment(envName string, baseDir string) (map[string]string, error)

// FindEnvironmentFile locates the file path for the named environment.
// Checks .yaml first, then .yml.
func FindEnvironmentFile(envName string, baseDir string) (string, error)

// ListAvailableEnvironments returns sorted names of environment files
// found in environments/ subdirectory of baseDir.
func ListAvailableEnvironments(baseDir string) []string
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseEnvironmentFile(t *testing.T) {
    tests := []struct {
        name     string
        content  string
        wantVars map[string]string
        wantErr  bool
    }{
        {"valid flat variables", "variables:\n  base_url: http://localhost", map[string]string{"base_url": "http://localhost"}, false},
        {"nested variables flattened", "variables:\n  db:\n    host: localhost\n    port: \"5432\"", map[string]string{"db_host": "localhost", "db_port": "5432"}, false},
        {"empty variables section", "variables:", map[string]string{}, false},
        {"no variables key", "other: value", map[string]string{}, false},
        {"invalid YAML", "{{invalid", nil, true},
        {"deeply nested", "variables:\n  a:\n    b:\n      c: deep", map[string]string{"a_b_c": "deep"}, false},
    }
}

func TestFindEnvironmentFile(t *testing.T) {
    tests := []struct {
        name      string
        files     []string // files to create in environments/
        envName   string
        wantFile  string
        wantErr   bool
    }{
        {"yaml extension found", []string{"dev.yaml"}, "dev", "dev.yaml", false},
        {"yml extension found", []string{"dev.yml"}, "dev", "dev.yml", false},
        {"yaml preferred over yml", []string{"dev.yaml", "dev.yml"}, "dev", "dev.yaml", false},
        {"not found", []string{"dev.yaml"}, "staging", "", true},
        {"no environments directory", nil, "dev", "", true},
    }
}

func TestListAvailableEnvironments(t *testing.T) {
    tests := []struct {
        name  string
        files []string
        want  []string
    }{
        {"multiple environments", []string{"dev.yaml", "staging.yml", "prod.yaml"}, []string{"dev", "prod", "staging"}},
        {"empty directory", nil, []string{}},
        {"no environments directory", nil, []string{}},
    }
}

func TestLoadEnvironment(t *testing.T) {
    tests := []struct {
        name     string
        envName  string
        // setup creates files and returns baseDir
        wantVars map[string]string
        wantErr  bool
    }{
        {"loads and parses environment", "dev", map[string]string{"base_url": "http://localhost"}, false},
        {"environment not found lists available", "missing", nil, true},
        {"invalid yaml reports file path", "bad", nil, true},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — new package

### Step 2: Update `runner.Run()` signature and variable merging
**Rationale:** Core logic change needed before CLI wiring. Mechanical updates to all call sites.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `envVars` parameter, implement precedence merging |
| `internal/runner/runner_test.go` | modify | Update all existing call sites, add precedence tests |

#### Current Code
```go
// runner.go line ~20
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, cliVars map[string]string) ([]RequestResult, *Summary, error) {
    // ...
    scope := variable.NewScope(col.Variables)
    if err := scope.Resolve(); err != nil {
        return nil, summary, err
    }
    // precedence 10 -- highest
    for k, v := range cliVars {
        scope.Set(k, v)
    }
```

#### New Code
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, envVars map[string]string, cliVars map[string]string) ([]RequestResult, *Summary, error) {
    // ...
    // Build merged variable map: env vars (level 3) < collection vars (level 7)
    merged := make(map[string]string, len(envVars)+len(col.Variables))
    for k, v := range envVars {
        merged[k] = v
    }
    for k, v := range col.Variables {
        merged[k] = v // collection overrides env
    }

    scope := variable.NewScope(merged)
    if err := scope.Resolve(); err != nil {
        return nil, summary, err
    }
    // CLI --var overrides (precedence 10 -- highest)
    for k, v := range cliVars {
        scope.Set(k, v)
    }
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_environment_variable_precedence(t *testing.T) {
    tests := []struct {
        name       string
        envVars    map[string]string
        colVars    map[string]string
        cliVars    map[string]string
        varName    string
        wantValue  string
    }{
        {"env var used when no collection var", map[string]string{"base_url": "http://env"}, nil, nil, "base_url", "http://env"},
        {"collection overrides env", map[string]string{"base_url": "http://env"}, map[string]string{"base_url": "http://col"}, nil, "base_url", "http://col"},
        {"cli overrides both", map[string]string{"base_url": "http://env"}, map[string]string{"base_url": "http://col"}, map[string]string{"base_url": "http://cli"}, "base_url", "http://cli"},
        {"nil envVars works", nil, map[string]string{"x": "1"}, nil, "x", "1"},
    }
}
```

#### Impact on Existing Tests
- **All existing `Run()` calls in `runner_test.go`** — need `nil` inserted before `cliVars` parameter. Mechanical fix.
- **`runCmd()` in `main.go`** — updated in Step 3.

### Step 3: Add `--env` flag to CLI
**Rationale:** Wires everything together — depends on Steps 1 and 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `--env` to `parseRunArgs()`, wire `config.LoadEnvironment()` in `runCmd()`, update help |
| `cmd/curlew/main_test.go` | modify | Add `--env` parsing tests, integration tests |
| `cmd/curlew/run_test.go` | modify | Update `runCmd()` call sites if any |

#### Current Code
```go
// parseRunArgs signature
func parseRunArgs(args []string) (file string, vars map[string]string, err error)
```

#### New Code
```go
func parseRunArgs(args []string) (file string, envName string, vars map[string]string, err error)
```

In `runCmd()`:
```go
file, envName, cliVars, err := parseRunArgs(args)
// ...
var envVars map[string]string
if envName != "" {
    baseDir := filepath.Dir(file)
    envVars, err = config.LoadEnvironment(envName, baseDir)
    if err != nil {
        return err
    }
}
results, summary, err := runner.Run(ctx, col, httpExec, envVars, cliVars)
```

Help text addition:
```
Run Options:
  --env <name>      Load environment file (from environments/<name>.yaml)
  --var key=value   Set a variable (overrides all other sources, repeatable)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs_env_flag(t *testing.T) {
    tests := []struct {
        name      string
        args      []string
        wantFile  string
        wantEnv   string
        wantVars  map[string]string
        wantErr   bool
    }{
        {"env flag", []string{"col.yaml", "--env", "dev"}, "col.yaml", "dev", nil, false},
        {"env with var", []string{"col.yaml", "--env", "dev", "--var", "x=1"}, "col.yaml", "dev", map[string]string{"x": "1"}, false},
        {"env without value", []string{"col.yaml", "--env"}, "", "", nil, true},
        {"no env flag", []string{"col.yaml"}, "col.yaml", "", nil, false},
    }
}

func TestCLIIntegration_env_flag(t *testing.T) {
    tests := []struct {
        name string
    }{
        {"env file loaded and variables used"},
        {"missing environment lists available"},
        {"invalid yaml reports file path"},
        {"collection overrides env variable"},
        {"cli var overrides all"},
        {"yml extension accepted"},
    }
}
```

#### Impact on Existing Tests
- All calls to `parseRunArgs()` in tests need updated return value handling
- All calls to `runCmd()` or integration tests — may need env setup

### Step 4: Update smoke test
**Rationale:** Final verification, after all code is working.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add `--env` smoke test section |

#### New Test Section
```bash
# Test: --env flag loads environment variables
mkdir -p "$TMPDIR/environments"
cat > "$TMPDIR/environments/dev.yaml" << 'YAML'
variables:
  base_url: "http://localhost:8080"
YAML
cat > "$TMPDIR/env-test.yaml" << 'YAML'
name: env test
requests:
  - name: test
    method: GET
    url: "{{base_url}}/health"
YAML
# Run with --env dev (will fail to connect, but should not error on variable resolution)
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All `Run()` calls (~30) | signature change | insert `nil` for `envVars` before `cliVars` |
| `cmd/curlew/main_test.go` | `TestParseRunArgs*` | signature change | add `envName` to return destructuring |
| `cmd/curlew/run_test.go` | Any `runCmd()` tests | may need env setup | update if present |

## Risks and Edge Cases

- **Risk:** Runner signature change breaks ~30 test calls → **Mitigation:** Mechanical search-and-replace, do in single commit
- **Risk:** Nested variable key collisions (e.g., `db_host` flat key AND `db: {host: ...}` nested) → **Mitigation:** Process nested first, flat keys overlay (last write wins)
- **Risk:** Variable name regex `[a-zA-Z_][a-zA-Z0-9_]*` doesn't support dots → **Mitigation:** Use underscore separator for flattening
- **Edge case:** No `environments/` directory → **Handling:** Structured error with hint to create the directory
- **Edge case:** Both `.yaml` and `.yml` for same name → **Handling:** Prefer `.yaml`, deterministic
- **Edge case:** Empty environment file → **Handling:** Valid, returns empty variable map
- **Edge case:** Environment file with no `variables` key → **Handling:** Valid, returns empty map

## TDD Commit Sequence

1. `test(config): add failing tests for environment file loading` (RED)
2. `feat(config): implement environment file parsing and discovery` (GREEN)
3. `test(runner): add failing tests for environment variable precedence` (RED)
4. `feat(runner): support environment variables with precedence` (GREEN)
5. `test(cli): add failing tests for --env flag` (RED)
6. `feat(cli): add --env flag with environment file loading` (GREEN)
7. `test(smoke): add --env smoke test`

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
mkdir -p environments
cat > environments/dev.yaml << 'YAML'
variables:
  base_url: "http://localhost:8080"
YAML
cat > env-test.yaml << 'YAML'
name: env test
requests:
  - name: test
    method: GET
    url: "{{base_url}}/health"
YAML
./curlew run env-test.yaml --env dev
```
