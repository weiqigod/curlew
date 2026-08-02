# Implementation Plan: M1-026

## Overview
Add an `exec` command to the CLI for single-request execution, supporting stdin JSON input, inline URL arguments, dry-run mode, JSONL logging, and non-interactive error handling. This is the primary command for AI-agent integration.

## Task Details
- **ID:** M1-026
- **Title:** AI exec command
- **Phase:** M1: Core CLI
- **Priority:** 26
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-020 | JSON output format | done |

## Implementation Steps

### Step 1: Add JSONL logging to `internal/output/`
**Rationale:** Smallest blast radius — new file, no existing code changes. Provides a reusable building block for the exec command.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/jsonl.go` | create | JSONLEntry struct and AppendJSONL function |
| `internal/output/jsonl_test.go` | create | Table-driven tests for JSONL logging |

#### New Code
```go
// internal/output/jsonl.go
package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSONLEntry is a single structured log entry for --log output.
type JSONLEntry struct {
	Timestamp  string `json:"timestamp"`
	Method     string `json:"method"`
	URL        string `json:"url"`
	StatusCode int    `json:"status_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

// AppendJSONL appends a single JSON-encoded line to the file at path.
// Creates the file if it does not exist.
func AppendJSONL(path string, entry *JSONLEntry) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encoding log entry: %w", err)
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("writing log entry: %w", err)
	}
	return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestAppendJSONL(t *testing.T) {
	tests := []struct {
		name string
		// ...
	}{
		{"creates file if not exists"},
		{"appends to existing file"},
		{"entry is valid JSON"},
		{"entry has newline terminator"},
		{"multiple entries are separate lines"},
		{"error fields omitted when empty"},
		{"dry_run field omitted when false"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Add `parseExecArgs` and `parseStdinRequest` with tests
**Rationale:** Parsing is foundational — needed before the command handler. Testable in isolation without HTTP dependencies.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `ExecOptions` struct, `parseExecArgs`, `parseStdinRequest` |
| `cmd/apitest/main_test.go` | modify | Add table-driven tests for both parse functions |

#### New Code
```go
// ExecOptions holds parsed arguments for the exec command.
type ExecOptions struct {
	URL            string
	Method         string
	Stdin          bool
	DryRun         bool
	LogFile        string
	Format         string
	NonInteractive bool
	NoColor        bool
	Verbosity      output.Verbosity
	Vars           map[string]string
	EnvVars        map[string]string
}

// parseExecArgs extracts flags and positional arguments for the exec command.
func parseExecArgs(args []string) (ExecOptions, error)

// stdinRequest is the JSON structure for --stdin input.
type stdinRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
	Query   map[string]string `json:"query"`
}

// parseStdinRequest reads JSON from r and returns a parser.Request.
func parseStdinRequest(r io.Reader) (*parser.Request, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseExecArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    ExecOptions
		wantErr string
	}{
		{"stdin flag sets stdin true", ...},
		{"dry run flag", ...},
		{"log file flag", ...},
		{"format json", ...},
		{"non interactive flag", ...},
		{"positional url", ...},
		{"method flag -X", ...},
		{"no args and no stdin returns error", ...},
		{"conflicting stdin and url returns error", ...},
		{"verbosity flags -v -vv -q", ...},
		{"var and env-var flags", ...},
		{"no-color flag", ...},
		{"log flag missing value returns error", ...},
		{"format flag missing value returns error", ...},
		{"method flag missing value returns error", ...},
	}
}

func TestParseStdinRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *parser.Request
		wantErr string
	}{
		{"valid json with url and method", ...},
		{"valid json with headers", ...},
		{"valid json with body object", ...},
		{"valid json with body string", ...},
		{"valid json with query params", ...},
		{"method defaults to GET when omitted", ...},
		{"missing url returns error", ...},
		{"invalid json returns error", ...},
		{"empty input returns error", ...},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected (new functions only)

---

### Step 3: Implement `execCmd` command handler
**Rationale:** Core command handler that ties together parsing, HTTP execution, output formatting, dry-run, and JSONL logging.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `execCmd` function |
| `cmd/apitest/main_test.go` | modify | Add tests for `execCmd` using in-process capture and httptest servers |

#### New Code
```go
// execCmd handles the "exec" subcommand.
// Exit codes: 0 = success, 1 = assertion failure, 3 = parse error, 4 = execution error.
func execCmd(args []string) int
```

Execution flow:
1. Parse args via `parseExecArgs`
2. Determine request source (stdin JSON or inline URL)
3. Build `*parser.Request` (apply variable interpolation for `--var`/`--env-var`)
4. If `--dry-run`: display request details (terminal or JSON), write JSONL if `--log`, exit 0
5. Execute via `httpexec.Execute(ctx, req)`
6. Build `JSONOutput` / terminal output reusing `buildJSONOutput` helper
7. If `--log`: append JSONL entry
8. Return exit code

For `--format json`, reuse `buildJSONOutput()` (already handles single-request results) and `output.WriteJSON()`. For terminal output, reuse `output.Printer`.

#### Tests to Write FIRST (RED phase)

```go
func TestExecCmd(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      string  // piped to stdin if non-empty
		wantExit   int
		wantStdout string // substring match
		wantStderr string // substring match
	}{
		{"stdin json executes request", ...},
		{"inline url executes request", ...},
		{"dry run does not execute http", ...},
		{"dry run shows request details", ...},
		{"format json produces valid json", ...},
		{"format json matches run schema", ...},
		{"invalid stdin json returns error", ...},
		{"non interactive suppresses prompts", ...},
		{"log appends jsonl entry", ...},
		{"dry run with log records dry run entry", ...},
		{"missing url and no stdin returns error", ...},
		{"var flag interpolates in url", ...},
		{"unknown format returns error", ...},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected (new function only)
- `captureRunCmd` helper only captures `runCmd`; need a new `captureExecCmd` or generalized helper

---

### Step 4: Wire exec into CLI switch and update help text
**Rationale:** Final wiring step — smallest blast radius since it's a single-line addition to the switch and help text.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `case "exec"` to switch, update `printHelp()` |
| `cmd/apitest/main_test.go` | modify | Add test for `run([]string{"exec", ...})` dispatch |

#### Current Code
```go
switch args[0] {
case "--version":
	// ...
case "run":
	return runCmd(args[1:])
// ...
default:
	// ...
}
```

#### New Code
```go
switch args[0] {
case "--version":
	// ...
case "run":
	return runCmd(args[1:])
case "exec":
	return execCmd(args[1:])
// ...
default:
	// ...
}
```

Help text additions:
```
Commands:
  ...
  exec <url>    Execute a single request (for AI agents and scripts)

Exec Options:
  --stdin             Read request JSON from stdin
  -X, --method <M>    HTTP method (default: GET)
  --dry-run           Show request details without executing
  --log <file>        Append structured JSONL log entry to file
  --non-interactive   Suppress interactive prompts on errors
  --format <type>     Output format: terminal (default), json
  --var key=value     Set a variable (repeatable)
  --env-var VAR       Import OS environment variable (repeatable)
  --no-color          Disable colored output
  -v / -vv / -q       Verbosity control
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_ExecDispatch(t *testing.T) {
	// Test that run([]string{"exec", "--help"}) or similar dispatches correctly
}

func TestHelpText_ContainsExec(t *testing.T) {
	// Verify "exec" appears in help output
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 5: Add integration test with real binary
**Rationale:** End-to-end verification using the built binary and stdin pipe.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main_test.go` | modify | Add integration tests using `buildBinary` + stdin pipe |

#### Tests to Write FIRST (RED phase)

```go
func TestExecIntegration(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantExit int
		check    func(t *testing.T, stdout, stderr string)
	}{
		{"stdin json via binary", ...},
		{"inline url via binary", ...},
		{"dry run via binary", ...},
		{"format json via binary", ...},
		{"invalid json via binary", ...},
		{"log file created via binary", ...},
	}
}
```

Uses a helper that pipes stdin to the binary:
```go
func runBinaryWithStdin(t *testing.T, binary, stdin string, args ...string) (stdout, stderr string, exitCode int)
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 6: Update smoke test
**Rationale:** Adds observable verification to the smoke test suite.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add exec command smoke tests |

#### New Smoke Tests
```bash
# Exec with stdin JSON
echo '{"url":"https://httpbin.org/get","method":"GET"}' | ./apitest exec --stdin

# Exec with inline URL
./apitest exec https://httpbin.org/get

# Exec with --dry-run
./apitest exec https://httpbin.org/get --dry-run

# Exec with --format json
echo '{"url":"https://httpbin.org/get","method":"GET"}' | ./apitest exec --stdin --format json

# Exec with invalid JSON (expect error)
echo 'not json' | ./apitest exec --stdin

# Exec with --log
echo '{"url":"https://httpbin.org/get","method":"GET"}' | ./apitest exec --stdin --log /tmp/test.jsonl

# Help text shows exec
./apitest --help | grep -q "exec"
```

#### Impact on Existing Tests
- No existing smoke tests affected

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/output/jsonl_test.go` | all | new | create |
| `cmd/apitest/main_test.go` | `TestParseExecArgs` | new | add |
| `cmd/apitest/main_test.go` | `TestParseStdinRequest` | new | add |
| `cmd/apitest/main_test.go` | `TestExecCmd` | new | add |
| `cmd/apitest/main_test.go` | `TestExecIntegration` | new | add |
| existing tests | — | none | — |

## Risks and Edge Cases

- **Risk:** Invalid JSON on stdin → **Mitigation:** Use `json.NewDecoder` for streaming; return clear error message "invalid JSON input: <details>"
- **Risk:** Empty stdin (EOF immediately) → **Mitigation:** Detect empty reader, return "no input received on stdin"
- **Risk:** Conflicting `--stdin` + positional URL → **Mitigation:** Reject at parse time: "cannot use --stdin with a URL argument"
- **Risk:** Large stdin input → **Mitigation:** Use `io.LimitReader` with 10MB cap to prevent memory exhaustion
- **Risk:** JSONL log file permission errors → **Mitigation:** `os.OpenFile` with `O_APPEND|O_CREATE|O_WRONLY`, propagate error with context
- **Edge case:** Dry-run with `--log` → **Handling:** Write JSONL entry with `"dry_run": true`, no status_code/duration
- **Edge case:** Method not specified (inline mode) → **Handling:** Default to GET
- **Edge case:** Method not specified (stdin JSON) → **Handling:** Default to GET
- **Edge case:** `--format json` output schema → **Handling:** Reuse `JSONOutput` with single request in `requests[]` array for schema compatibility with `run` command
- **Edge case:** `--non-interactive` with error → **Handling:** Suppress hints/suggestions, output only the error message
- **Edge case:** Variable interpolation in exec mode → **Handling:** `--var` and `--env-var` flags work, interpolate URL/headers/body before execution

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
echo '{"url":"https://httpbin.org/get","method":"GET"}' | ./apitest exec --stdin --format json
```
