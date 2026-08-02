# Implementation Plan: M1-013

## Overview
Add `.env` file auto-loading so local secrets (like API keys) in `KEY=VALUE` format are automatically available as `{{KEY}}` variables, sitting between environment file and collection-level variable precedence.

## Task Details
- **ID:** M1-013
- **Title:** .env file loading for local secrets
- **Phase:** M1: Core CLI
- **Priority:** 13
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-012 | Environment files with --env flag | done |

## Implementation Steps

### Step 1: ParseDotenv and LoadDotenv in internal/config/
**Rationale:** Smallest blast radius — new files only, no existing code changes. The parser is a pure function that can be fully tested in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/dotenv.go` | create | .env file parser and loader |
| `internal/config/dotenv_test.go` | create | Table-driven tests for parsing and loading |

#### New Code
```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrInvalidDotenv indicates the .env file contains malformed lines.
var ErrInvalidDotenv = errors.New("invalid .env file")

// ParseDotenv parses .env format content from a byte slice.
// Lines starting with '#' are comments. Empty lines are ignored.
// Values may be optionally quoted (single or double quotes stripped).
// Returns ErrInvalidDotenv for malformed lines (no '=' or empty key).
func ParseDotenv(data []byte) (map[string]string, error) {
	result := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			return nil, fmt.Errorf("%w: line %d: %q", ErrInvalidDotenv, i+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:eqIdx])
		if key == "" {
			return nil, fmt.Errorf("%w: line %d: empty key", ErrInvalidDotenv, i+1)
		}
		value := strings.TrimSpace(trimmed[eqIdx+1:])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		result[key] = value
	}
	return result, nil
}

// LoadDotenv loads a .env file from dir. Returns empty map and nil error
// if the file does not exist. Returns error if file exists but cannot be parsed.
func LoadDotenv(dir string) (map[string]string, error) {
	path := filepath.Join(dir, ".env")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, fmt.Errorf("reading .env file: %w", err)
	}
	return ParseDotenv(data)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseDotenv(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{"simple key=value", "KEY=value", map[string]string{"KEY": "value"}, false},
		{"multiple pairs", "A=1\nB=2", map[string]string{"A": "1", "B": "2"}, false},
		{"comment lines ignored", "# comment\nKEY=val", map[string]string{"KEY": "val"}, false},
		{"empty lines ignored", "\nKEY=val\n\n", map[string]string{"KEY": "val"}, false},
		{"whitespace-only lines ignored", "  \nKEY=val", map[string]string{"KEY": "val"}, false},
		{"empty value", "KEY=", map[string]string{"KEY": ""}, false},
		{"value with equals sign", "KEY=a=b=c", map[string]string{"KEY": "a=b=c"}, false},
		{"double-quoted value", `KEY="value with spaces"`, map[string]string{"KEY": "value with spaces"}, false},
		{"single-quoted value", "KEY='value with spaces'", map[string]string{"KEY": "value with spaces"}, false},
		{"quoted value preserves inner spaces", `KEY="  spaced  "`, map[string]string{"KEY": "  spaced  "}, false},
		{"inline hash not treated as comment", "KEY=value#notcomment", map[string]string{"KEY": "value#notcomment"}, false},
		{"empty input", "", map[string]string{}, false},
		{"only comments and blanks", "# comment\n\n# another", map[string]string{}, false},
		{"leading/trailing whitespace on key trimmed", "  KEY  =value", map[string]string{"KEY": "value"}, false},
		{"leading/trailing whitespace on unquoted value trimmed", "KEY=  value  ", map[string]string{"KEY": "value"}, false},
		{"quoted value does not trim internal spaces", `KEY="  value  "`, map[string]string{"KEY": "  value  "}, false},
		{"line with no equals sign", "BADLINE", nil, true},
		{"empty key", "=value", nil, true},
		{"duplicate keys last wins", "KEY=first\nKEY=second", map[string]string{"KEY": "second"}, false},
		{"URL as value", "URL=https://example.com/api?q=1&x=2", map[string]string{"URL": "https://example.com/api?q=1&x=2"}, false},
		{"windows line endings", "A=1\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}, false},
	}
	// ...
}

func TestLoadDotenv(t *testing.T) {
	tests := []struct {
		name      string
		content   *string  // nil = don't create file
		wantCount int
		wantErr   bool
	}{
		{"file exists with valid content", ptr("KEY=val"), 1, false},
		{"file does not exist", nil, 0, false},
		{"file is empty", ptr(""), 0, false},
		{"invalid content", ptr("BADLINE"), 0, true},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new files only)

---

### Step 2: Add dotenvVars parameter to runner.Run()
**Rationale:** Modifies the core function signature — must come after the parser exists so the new tests can verify precedence. Mechanical update to all call sites.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `dotenvVars` parameter, update merge logic |
| `internal/runner/runner_test.go` | modify | Update ~44 call sites, add precedence tests |

#### Current Code
```go
// runner.go lines 38-50
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, envVars, cliVars map[string]string) ([]RequestResult, *Summary, error) {
	results := make([]RequestResult, 0, len(col.Requests))
	summary := &Summary{Total: len(col.Requests)}

	// Build merged variable map: env vars (level 3) < collection vars (level 7)
	merged := make(map[string]string, len(envVars)+len(col.Variables))
	for k, v := range envVars {
		merged[k] = v
	}
	for k, v := range col.Variables {
		merged[k] = v // collection overrides env
	}
```

#### New Code
```go
// runner.go — updated signature and merge logic
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, envVars, dotenvVars, cliVars map[string]string) ([]RequestResult, *Summary, error) {
	results := make([]RequestResult, 0, len(col.Requests))
	summary := &Summary{Total: len(col.Requests)}

	// Build merged variable map: env (level 3) < .env (level 4) < collection (level 7)
	merged := make(map[string]string, len(envVars)+len(dotenvVars)+len(col.Variables))
	for k, v := range envVars {
		merged[k] = v
	}
	for k, v := range dotenvVars {
		merged[k] = v // .env overrides environment file
	}
	for k, v := range col.Variables {
		merged[k] = v // collection overrides .env
	}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_dotenv_used_when_no_other_vars(t *testing.T) {
	// Only dotenv vars provided, verify interpolation works
}

func TestRun_dotenv_overrides_env(t *testing.T) {
	// Both env and dotenv define same key, dotenv wins
}

func TestRun_collection_overrides_dotenv(t *testing.T) {
	// Both dotenv and collection define same key, collection wins
}

func TestRun_cli_overrides_dotenv(t *testing.T) {
	// Both dotenv and CLI define same key, CLI wins
}

func TestRun_full_precedence_chain(t *testing.T) {
	// All four sources define same key, verify CLI > collection > dotenv > env
}

func TestRun_nil_dotenv_vars_works(t *testing.T) {
	// Pass nil for dotenvVars, verify no panic
}
```

#### Impact on Existing Tests
- **~44 call sites** in `runner_test.go` need a `nil` parameter inserted for `dotenvVars`:
  - `Run(..., nil, nil)` → `Run(..., nil, nil, nil)` (majority of calls)
  - `Run(..., envVars, nil)` → `Run(..., envVars, nil, nil)` (2 calls)
  - `Run(..., nil, cliVars)` → `Run(..., nil, nil, cliVars)` (3 calls)
  - `Run(..., envVars, cliVars)` → `Run(..., envVars, nil, cliVars)` (1 call)
- All existing tests pass unchanged after the mechanical parameter insertion.

---

### Step 3: Wire .env loading in main.go
**Rationale:** Final wiring step — depends on both the parser (Step 1) and the runner signature (Step 2).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add .env loading before runner.Run(), update help text |

#### Current Code
```go
// main.go lines 96-110
	// Load environment variables if --env specified
	var envVars map[string]string
	if envName != "" {
		baseDir := filepath.Dir(file)
		envVars, err = config.LoadEnvironment(envName, baseDir)
		if err != nil {
			output.PrintStructuredError(os.Stderr, err)
			return 3
		}
	}

	ctx := context.Background()
	output.PrintCollectionHeader(os.Stdout, col.Name)

	results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, envVars, cliVars)
```

#### New Code
```go
	// Load environment variables if --env specified
	var envVars map[string]string
	if envName != "" {
		baseDir := filepath.Dir(file)
		envVars, err = config.LoadEnvironment(envName, baseDir)
		if err != nil {
			output.PrintStructuredError(os.Stderr, err)
			return 3
		}
	}

	// Load .env variables from project root (optional, no error if missing)
	dotenvVars, dotenvErr := config.LoadDotenv(filepath.Dir(file))
	if dotenvErr != nil {
		output.PrintStructuredError(os.Stderr, dotenvErr)
		return 3
	}

	ctx := context.Background()
	output.PrintCollectionHeader(os.Stdout, col.Name)

	results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, envVars, dotenvVars, cliVars)
```

#### Help text update
```go
	fmt.Println("Run Options:")
	fmt.Println("  --env <name>      Load environment file (from environments/<name>.yaml)")
	fmt.Println("  --var key=value   Set a variable (overrides all other sources, repeatable)")
	fmt.Println()
	fmt.Println("Auto-loaded:")
	fmt.Println("  .env              Local secrets (KEY=VALUE format, optional)")
```

#### Impact on Existing Tests
- No dedicated tests for `main.go` (tested via smoke tests)

---

### Step 4: Smoke test update
**Rationale:** Validates the full end-to-end flow after all wiring is complete.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add .env auto-loading smoke test |

#### New smoke test section
```bash
echo "--- Running collection with .env auto-loading (expect pass) ---"
DOTENV_DIR=$(mktemp -d /tmp/curlew_dotenv_XXXXXX)
cat > "$DOTENV_DIR/.env" << 'ENV'
BASE_URL=https://httpbin.org
ENV
cat > "$DOTENV_DIR/test.yaml" << 'YAML'
name: Dotenv Test
requests:
  - name: Check Dotenv
    request:
      method: GET
      url: "{{BASE_URL}}/get"
    assertions:
      status: 200
YAML
./curlew run "$DOTENV_DIR/test.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$DOTENV_DIR"
echo

echo "--- Running collection without .env (expect pass, no error) ---"
NO_DOTENV_DIR=$(mktemp -d /tmp/curlew_no_dotenv_XXXXXX)
cat > "$NO_DOTENV_DIR/test.yaml" << 'YAML'
name: No Dotenv Test
requests:
  - name: Check No Dotenv
    request:
      method: GET
      url: "https://httpbin.org/get"
    assertions:
      status: 200
YAML
./curlew run "$NO_DOTENV_DIR/test.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$NO_DOTENV_DIR"
echo
```

---

### Step 5: CHANGELOG update
**Rationale:** Required by definition of done.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add .env entry under Unreleased / Added |

#### New entry (after first Added line)
```markdown
- `.env` file auto-loading for local secrets: `KEY=VALUE` format, comments, quoted values, optional (no error if missing); precedence: environment < .env < collection < CLI `--var` (M1-013)
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All `Run()` call sites (~44) | breaks (missing parameter) | Insert `nil` for `dotenvVars` |
| `internal/config/dotenv_test.go` | — | new file | Write tests first (TDD) |
| `cmd/curlew/main.go` | — | recompile | Smoke test validates |

## Risks and Edge Cases
- **Risk:** Runner signature change breaks all existing call sites → **Mitigation:** Mechanical update (insert `nil`), all within this project. Compile-time error ensures nothing is missed.
- **Edge case:** Malformed .env line (no `=`) → **Handling:** Return `ErrInvalidDotenv` with line number and content in message.
- **Edge case:** Empty key (`=value`) → **Handling:** Return `ErrInvalidDotenv` with line number.
- **Edge case:** Mismatched quotes (`KEY="value`) → **Handling:** Treat as unquoted — only matched surrounding quotes are stripped.
- **Edge case:** Windows line endings (`\r\n`) → **Handling:** `strings.TrimRight(line, "\r")` before parsing.
- **Edge case:** Duplicate keys → **Handling:** Last value wins (standard dotenv behavior).
- **Edge case:** No `.env` file → **Handling:** Return empty map and nil error (file is optional).
- **Edge case:** BOM (byte order mark) in .env file → **Handling:** Not handled in v1. Can be added later if needed.
- **Edge case:** `export` prefix (`export KEY=value`) → **Handling:** Not supported (spec does not mention it). Can be added later.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create test directory
mkdir -p /tmp/dotenv-test
echo 'API_KEY=secret123' > /tmp/dotenv-test/.env
cat > /tmp/dotenv-test/test.yaml << 'YAML'
name: Dotenv Observable
requests:
  - name: Check
    request:
      method: GET
      url: "https://httpbin.org/get"
      headers:
        X-Api-Key: "{{API_KEY}}"
    assertions:
      status: 200
YAML
./curlew run /tmp/dotenv-test/test.yaml
rm -rf /tmp/dotenv-test
```
