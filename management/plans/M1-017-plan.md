# Implementation Plan: M1-017

## Overview
Adds `curlew.yaml` project-level configuration: parse the file, detect the project root by walking up the directory tree, load global variables at precedence level 2 (lowest, below environment files), and expose `project_name` for output context.

## Task Details
- **ID:** M1-017
- **Title:** Global project config (curlew.yaml)
- **Phase:** M1: Core CLI
- **Priority:** 17
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-012 | Environment files (--env flag) | done |

## Clarification: Variable Precedence

The task description says "collection (level 2) equals project config (level 2)". This is incorrect per the specification. From `docs/SPECIFICATION.md`:

```
Precedence (lowest to highest):
1. Dynamic functions ({{$timestamp}}, {{$uuid}})
2. Global variables (curlew.yaml)          ← this task
3. Environment variables (environments/dev.yaml)
4. Local secrets (.env)
7. Collection-level variables
8. Request-level variables
9. CLI --env-var values
10. CLI --var values (highest)
```

curlew.yaml variables are **level 2** — below env files (3), dotenv (4), and collection variables (7). Collection wins because it is at level 7, not because they share a level.

Also: per the spec, `.env` should be loaded from the **project root** (the directory containing `curlew.yaml`). Currently it loads from the collection directory. When a project root is found, this task corrects that.

## Implementation Steps

### Step 1: Add `ProjectConfig` struct and `ParseProjectConfig` in `internal/config/project.go`

**Rationale:** Pure addition — no existing code touched. Establishes the data type and lowest-blast-radius piece first.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | create | ProjectConfig struct, ErrInvalidProjectConfig, ParseProjectConfig |
| `internal/config/project_test.go` | create | Table-driven tests for ParseProjectConfig |

#### New Code

```go
// internal/config/project.go

package config

import (
    "errors"
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

// ErrInvalidProjectConfig is returned when curlew.yaml cannot be parsed.
var ErrInvalidProjectConfig = errors.New("invalid project config")

// ProjectConfig holds the parsed contents of an curlew.yaml file.
type ProjectConfig struct {
    ProjectName string
    Variables   map[string]string
}

type projectFile struct {
    ProjectName string         `yaml:"project_name"`
    Variables   map[string]any `yaml:"variables"`
}

// ParseProjectConfig reads and parses an curlew.yaml file at the given path.
// Returns ErrInvalidProjectConfig for parse failures.
func ParseProjectConfig(path string) (*ProjectConfig, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read project config: %w", err)
    }
    var pf projectFile
    if err := yaml.Unmarshal(data, &pf); err != nil {
        return nil, fmt.Errorf("%w: %s", ErrInvalidProjectConfig, err)
    }
    return &ProjectConfig{
        ProjectName: pf.ProjectName,
        Variables:   flatten(pf.Variables),
    }, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseProjectConfig(t *testing.T) {
    tests := []struct {
        name        string
        yaml        string
        wantProject string
        wantVars    map[string]string
        wantErr     error
    }{
        {
            name:        "valid with project_name and variables",
            yaml:        "project_name: MyApp\nvariables:\n  base_url: https://api.example.com\n  api_version: v1\n",
            wantProject: "MyApp",
            wantVars:    map[string]string{"base_url": "https://api.example.com", "api_version": "v1"},
        },
        {
            name:        "valid with project_name only",
            yaml:        "project_name: MyApp\n",
            wantProject: "MyApp",
            wantVars:    map[string]string{},
        },
        {
            name:     "valid with variables only",
            yaml:     "variables:\n  key: value\n",
            wantVars: map[string]string{"key": "value"},
        },
        {
            name:     "empty file returns zero struct",
            yaml:     "",
            wantVars: map[string]string{},
        },
        {
            name:    "invalid yaml returns ErrInvalidProjectConfig",
            yaml:    "variables: [not: a: map]",
            wantErr: ErrInvalidProjectConfig,
        },
    }
    // ...
}
```

#### Impact on Existing Tests
None — new file only.

---

### Step 2: Add `FindProjectRoot` in `internal/config/project.go`

**Rationale:** Depends on knowing the filename (`curlew.yaml`). Independent of existing runner/parser code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add FindProjectRoot function |
| `internal/config/project_test.go` | modify | Add TestFindProjectRoot cases |

#### New Code

```go
// FindProjectRoot walks up from startDir looking for curlew.yaml (or curlew.yml).
// Returns the directory containing the project config file and true,
// or ("", false) if no project config is found up to the filesystem root.
func FindProjectRoot(startDir string) (string, bool) {
    dir := startDir
    for {
        for _, name := range []string{"curlew.yaml", "curlew.yml"} {
            if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
                return dir, true
            }
        }
        parent := filepath.Dir(dir)
        if parent == dir { // reached filesystem root
            return "", false
        }
        dir = parent
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestFindProjectRoot(t *testing.T) {
    tests := []struct {
        name      string
        setup     func(t *testing.T) string // returns startDir
        wantFound bool
    }{
        {
            name:      "found in same directory",
            setup:     func(t *testing.T) string { /* create curlew.yaml in temp dir, return temp dir */ },
            wantFound: true,
        },
        {
            name:      "found in parent directory",
            setup:     func(t *testing.T) string { /* curlew.yaml in parent, return child */ },
            wantFound: true,
        },
        {
            name:      "found in grandparent directory",
            setup:     func(t *testing.T) string { /* curlew.yaml two levels up */ },
            wantFound: true,
        },
        {
            name:      "not found returns false",
            setup:     func(t *testing.T) string { /* temp dir with no curlew.yaml */ },
            wantFound: false,
        },
        {
            name:      "curlew.yml extension supported",
            setup:     func(t *testing.T) string { /* create curlew.yml, not curlew.yaml */ },
            wantFound: true,
        },
    }
    // ...
}
```

#### Impact on Existing Tests
None — new function only.

---

### Step 3: Add `LoadProjectConfig` convenience function

**Rationale:** Mirrors the pattern of `LoadEnvironment` and `LoadDotenv` — high-level function combining discovery and parsing. Depends on steps 1 and 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add LoadProjectConfig function |
| `internal/config/project_test.go` | modify | Add TestLoadProjectConfig cases |

#### New Code

```go
// LoadProjectConfig finds the project root by walking up from startDir,
// then parses curlew.yaml (or curlew.yml). Returns an empty ProjectConfig and
// empty root string if no project config is found (not an error). Returns error
// if the file exists but cannot be parsed.
func LoadProjectConfig(startDir string) (*ProjectConfig, string, error) {
    root, found := FindProjectRoot(startDir)
    if !found {
        return &ProjectConfig{Variables: map[string]string{}}, "", nil
    }
    name := "curlew.yaml"
    if _, err := os.Stat(filepath.Join(root, name)); errors.Is(err, os.ErrNotExist) {
        name = "curlew.yml"
    }
    cfg, err := ParseProjectConfig(filepath.Join(root, name))
    if err != nil {
        return nil, "", err
    }
    return cfg, root, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestLoadProjectConfig(t *testing.T) {
    tests := []struct {
        name         string
        setup        func(t *testing.T) string
        wantVars     map[string]string
        wantRootSet  bool
        wantErr      error
    }{
        {"loads from project root", ..., wantRootSet: true},
        {"no project root returns empty config no error", ..., wantRootSet: false},
        {"invalid yaml returns error", ..., wantErr: ErrInvalidProjectConfig},
        {"yml extension supported", ..., wantRootSet: true},
    }
    // ...
}
```

#### Impact on Existing Tests
None — new function only.

---

### Step 4: Add `Project` field to `VarSources` and integrate into `runner.Run`

**Rationale:** First modification to existing code. Adds the project variable layer to the precedence chain. `VarSources{}` zero value (nil `Project`) is safe — iterating nil map is a no-op in Go.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add Project field to VarSources; insert into merge chain before EnvFile |
| `internal/runner/runner_test.go` | modify | Add tests for project variable precedence |

#### Current Code

```go
// VarSources holds variable maps at each precedence level.
type VarSources struct {
    EnvFile map[string]string // precedence 3: --env environment file
    DotEnv  map[string]string // precedence 4: .env file
    EnvVar  map[string]string // precedence 9: --env-var flags
    CLI     map[string]string // precedence 10: --var flags
}
```

```go
// In Run(), merge order:
// Precedence: EnvFile (3) < DotEnv (4) < collection (7) < EnvVar (9) < CLI (10).
merged := make(map[string]string)
for k, v := range sources.EnvFile {
    merged[k] = v
}
for k, v := range sources.DotEnv {
    merged[k] = v
}
```

#### New Code

```go
// VarSources holds variable maps at each precedence level.
type VarSources struct {
    Project map[string]string // precedence 2: curlew.yaml global variables
    EnvFile map[string]string // precedence 3: --env environment file
    DotEnv  map[string]string // precedence 4: .env file
    EnvVar  map[string]string // precedence 9: --env-var flags
    CLI     map[string]string // precedence 10: --var flags
}
```

```go
// In Run(), merge order:
// Precedence: Project (2) < EnvFile (3) < DotEnv (4) < collection (7) < EnvVar (9) < CLI (10).
merged := make(map[string]string)
for k, v := range sources.Project {
    merged[k] = v
}
for k, v := range sources.EnvFile {
    merged[k] = v
}
for k, v := range sources.DotEnv {
    merged[k] = v
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_ProjectVariables(t *testing.T) {
    tests := []struct {
        name       string
        project    map[string]string
        envFile    map[string]string
        collection map[string]string
        cli        map[string]string
        requestURL string // template using a variable
        wantURL    string
    }{
        {
            name:       "project variables available in interpolation",
            project:    map[string]string{"base_url": "https://project.example.com"},
            requestURL: "{{base_url}}/path",
            wantURL:    "https://project.example.com/path",
        },
        {
            name:       "env file overrides project",
            project:    map[string]string{"base_url": "https://project.example.com"},
            envFile:    map[string]string{"base_url": "https://env.example.com"},
            requestURL: "{{base_url}}/path",
            wantURL:    "https://env.example.com/path",
        },
        {
            name:       "collection overrides project",
            project:    map[string]string{"base_url": "https://project.example.com"},
            collection: map[string]string{"base_url": "https://collection.example.com"},
            requestURL: "{{base_url}}/path",
            wantURL:    "https://collection.example.com/path",
        },
        {
            name:       "cli overrides project",
            project:    map[string]string{"base_url": "https://project.example.com"},
            cli:        map[string]string{"base_url": "https://cli.example.com"},
            requestURL: "{{base_url}}/path",
            wantURL:    "https://cli.example.com/path",
        },
        {
            name:       "nil project map is safe (no panic)",
            project:    nil,
            requestURL: "https://static.example.com/path",
            wantURL:    "https://static.example.com/path",
        },
    }
    // ...
}
```

#### Impact on Existing Tests
None. All existing `VarSources{}` literals omit `Project`, which defaults to `nil`. Iterating a nil map is a no-op in Go — existing behavior unchanged.

---

### Step 5: Wire up in `cmd/curlew/main.go`

**Rationale:** Final integration. All underlying pieces are tested. This connects real file discovery to the runner.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Call LoadProjectConfig in runCmd; update .env lookup dir; pass project vars to VarSources |
| `cmd/curlew/main_test.go` | modify | Add integration tests for project config |

#### Current Code (in `runCmd`)

```go
dotEnvVars, err := config.LoadDotenv(filepath.Dir(file))
if err != nil {
    return fmt.Errorf("load .env: %w", err)
}
// ...
sources := runner.VarSources{
    EnvFile: envVars,
    DotEnv:  dotEnvVars,
    EnvVar:  envVarMap,
    CLI:     cliVarMap,
}
```

#### New Code

```go
collectionDir := filepath.Dir(file)

projectCfg, projectRoot, err := config.LoadProjectConfig(collectionDir)
if err != nil {
    return fmt.Errorf("load project config: %w", err)
}

// Load .env from project root if found, otherwise from collection directory.
dotEnvDir := collectionDir
if projectRoot != "" {
    dotEnvDir = projectRoot
}
dotEnvVars, err := config.LoadDotenv(dotEnvDir)
if err != nil {
    return fmt.Errorf("load .env: %w", err)
}
// ...
sources := runner.VarSources{
    Project: projectCfg.Variables,
    EnvFile: envVars,
    DotEnv:  dotEnvVars,
    EnvVar:  envVarMap,
    CLI:     cliVarMap,
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_ProjectConfig(t *testing.T) {
    // Integration tests using real temp directories.
    tests := []struct {
        name    string
        setup   func(t *testing.T) (collectionFile string)
        wantErr bool
    }{
        {
            name: "project variables resolved in collection",
            // curlew.yaml in collectionDir with base_url; collection uses {{base_url}}
        },
        {
            name: "no curlew.yaml runs without error",
            // no curlew.yaml anywhere, collection with static URL
        },
        {
            name: "project config in parent directory (walk-up)",
            // curlew.yaml in parent, collection in subdirectory
        },
        {
            name: "project variables overridden by collection variables",
            // both curlew.yaml and collection define same key; collection wins
        },
        {
            name: "invalid curlew.yaml returns error",
            // malformed curlew.yaml in collectionDir
        },
        {
            name: "dot env loaded from project root not collection dir",
            // curlew.yaml in parent, .env in parent (not in collectionDir), collection uses secret var
        },
    }
    // ...
}
```

#### Impact on Existing Tests
None. Existing tests run collections in temp directories without `curlew.yaml`. `LoadProjectConfig` returns empty config with nil error when no project config is found.

---

### Step 6: Update help text and smoke test

**Rationale:** Definition of Done requires help text and smoke test to reflect the new capability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add curlew.yaml to Auto-loaded section in printHelp() |
| `smoke/run.sh` | modify | Add smoke test case for project config walk-up |

#### Help Text Change

```
Auto-loaded:
  curlew.yaml          Project config with global variables (optional, walks up from collection dir)
  .env                  Local secrets (KEY=VALUE format, optional)
```

#### Smoke Test Addition

```bash
# Test: project config variables resolved
# Setup: curlew.yaml in parent, collection in subdirectory
mkdir -p "$TMP/project/sub"
cat > "$TMP/project/curlew.yaml" <<EOF
project_name: SmokeTest
variables:
  smoke_base: https://httpbin.org
EOF
cat > "$TMP/project/sub/collection.yaml" <<EOF
name: ProjectConfigTest
requests:
  - name: Test Request
    method: GET
    url: "{{smoke_base}}/get"
EOF
run_test "project config walk-up" "$TMP/project/sub/collection.yaml"
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All existing | none | No change needed |
| `cmd/curlew/main_test.go` | All existing | none | No change needed |
| `internal/config/*_test.go` | All existing | none | No change needed |

## Risks and Edge Cases

- **Infinite loop in `FindProjectRoot`**: `filepath.Dir()` returns the same value at filesystem root. Detect by comparing `dir == parent` before updating. ✓ Handled in proposed code.
- **`.env` location change**: When a project root is found, `.env` loading shifts from the collection dir to the project root. Existing tests are unaffected (no `curlew.yaml` in temp dirs). Document this clearly.
- **nil `Project` map in `VarSources`**: Iterating a nil map in Go is safe (no-op). Zero-value `VarSources{}` is still correct.
- **`curlew.yaml` vs `curlew.yml`**: Check `.yaml` first (canonical), then `.yml`. Both `FindProjectRoot` and `LoadProjectConfig` handle this.
- **Reusing `flatten()`**: The `flatten()` function is in `environment.go` in the same package — directly reusable without export.
- **`project_name` usage**: Behavior 4 says "available for output context". For this task, store it in `ProjectConfig.ProjectName`. The output package currently uses `col.Name`. No display change is needed now — the field just needs to be parsed and accessible.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create project directory structure
mkdir -p /tmp/curlew-demo/requests
cat > /tmp/curlew-demo/curlew.yaml <<EOF
project_name: DemoProject
variables:
  base_url: https://httpbin.org
EOF
cat > /tmp/curlew-demo/requests/collection.yaml <<EOF
name: Demo
requests:
  - name: Get
    method: GET
    url: "{{base_url}}/get"
EOF
./curlew run /tmp/curlew-demo/requests/collection.yaml
# Expected: request succeeds, no "undefined variable" errors
```
