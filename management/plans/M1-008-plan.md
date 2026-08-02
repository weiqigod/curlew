# Implementation Plan: M1-008

## Overview

Enhance error handling across parser, HTTP executor, and output packages to produce structured, classified errors with consistent `[ERROR] file:line — message` formatting and actionable hints.

## Task Details
- **ID:** M1-008
- **Title:** Structured error messages (parse, network, config)
- **Phase:** M1: Core CLI
- **Priority:** 8
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-004 | Assert on status code | done |

## Implementation Steps

### Step 1: Create `internal/errors` package with structured error types

**Rationale:** Foundation package with zero existing callers. Can be added without breaking anything. All subsequent steps depend on it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/errors/errors.go` | create | Structured error types, network error classification, Format function |
| `internal/errors/errors_test.go` | create | Tests for all error types, classification, and formatting |

#### New Code

```go
package errors

import (
    "crypto/tls"
    "crypto/x509"
    "errors"
    "fmt"
    "net"
    "net/url"
    "strings"
    "time"
)

// Category classifies error types for consistent formatting.
type Category string

const (
    CategoryParse   Category = "parse"
    CategoryNetwork Category = "network"
    CategoryConfig  Category = "config"
)

// Structured carries location context for user-facing error display.
type Structured struct {
    Category Category
    FilePath string
    Line     int    // 0 = unknown
    Message  string
    Hint     string // optional fix suggestion
    Inner    error  // for errors.Is/As chain
}

func (e *Structured) Error() string {
    if e.FilePath != "" && e.Line > 0 {
        return fmt.Sprintf("%s:%d: %s", e.FilePath, e.Line, e.Message)
    }
    if e.FilePath != "" {
        return fmt.Sprintf("%s: %s", e.FilePath, e.Message)
    }
    return e.Message
}

func (e *Structured) Unwrap() error { return e.Inner }

// NetworkErrorKind classifies network-level failures.
type NetworkErrorKind int

const (
    NetworkDNS NetworkErrorKind = iota
    NetworkConnectionRefused
    NetworkTLS
    NetworkTimeout
    NetworkOther
)

// NetworkError is a classified network failure.
type NetworkError struct {
    Kind     NetworkErrorKind
    Host     string
    Duration time.Duration // meaningful for timeouts
    Message  string
    Hint     string
    Inner    error
}

func (e *NetworkError) Error() string { return e.Message }
func (e *NetworkError) Unwrap() error { return e.Inner }

// ClassifyNetworkError inspects a Go net/http error chain and returns
// a classified NetworkError with user-facing message and hint.
func ClassifyNetworkError(err error) *NetworkError { ... }

// Format returns a display string for any error.
// Structured: "[ERROR] file:line — message\n  hint"
// NetworkError: "[ERROR] message\n  hint"
// Plain: "[ERROR] message"
func Format(err error) string { ... }
```

#### Tests to Write FIRST (RED phase)

```go
func TestStructuredError(t *testing.T) {
    tests := []struct {
        name string
        err  *Structured
        want string
    }{
        {"with file and line", &Structured{FilePath: "test.yaml", Line: 5, Message: "bad syntax"}, "test.yaml:5: bad syntax"},
        {"with file no line", &Structured{FilePath: "test.yaml", Line: 0, Message: "empty"}, "test.yaml: empty"},
        {"no file", &Structured{Message: "something failed"}, "something failed"},
    }
    ...
}

func TestStructuredUnwrap(t *testing.T) {
    // errors.Is works through chain
}

func TestClassifyNetworkError(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        wantKind NetworkErrorKind
    }{
        {"dns failure", &net.DNSError{...}, NetworkDNS},
        {"connection refused", <opError with ECONNREFUSED>, NetworkConnectionRefused},
        {"tls error", <tls verification error>, NetworkTLS},
        {"timeout - context deadline", context.DeadlineExceeded, NetworkTimeout},
        {"other network error", errors.New("unknown"), NetworkOther},
    }
    ...
}

func TestNetworkErrorHints(t *testing.T) {
    tests := []struct {
        name     string
        kind     NetworkErrorKind
        wantHint string
    }{
        {"dns hint", NetworkDNS, "Check that the hostname is correct"},
        {"connection refused hint", NetworkConnectionRefused, "Check that the server is running"},
        {"tls hint", NetworkTLS, "certificate"},
        {"timeout hint", NetworkTimeout, "increasing the timeout"},
    }
    ...
}

func TestFormat(t *testing.T) {
    tests := []struct {
        name string
        err  error
        want string
    }{
        {"structured with file and line", &Structured{FilePath: "t.yaml", Line: 5, Message: "bad"}, "[ERROR] t.yaml:5 — bad"},
        {"structured with hint", &Structured{FilePath: "t.yaml", Line: 5, Message: "bad", Hint: "fix it"}, "[ERROR] t.yaml:5 — bad\n  Hint: fix it"},
        {"plain error", errors.New("boom"), "[ERROR] boom"},
        {"network error with hint", &NetworkError{Message: "DNS failed", Hint: "check host"}, "[ERROR] DNS failed\n  Hint: check host"},
    }
    ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

---

### Step 2: Enhance parser to produce structured errors with file path and line number

**Rationale:** Parser errors are simplest to enhance — `yaml.v3` already provides line numbers. Changes `ParseFile` to return `*errors.Structured`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/errors.go` | modify | Add `ErrMissingRequiredField` sentinel |
| `internal/parser/parser.go` | modify | Wrap errors in `Structured`, add required-field validation |
| `internal/parser/parser_test.go` | modify | Add tests for structured error output, add missing-field tests |

#### Current Code

```go
// parser.go - ParseFile error handling
return nil, fmt.Errorf("%w: %s", ErrFileNotFound, path)
// ...
return nil, fmt.Errorf("%w: %w", ErrInvalidYAML, err)
// ...
return nil, fmt.Errorf("%w: %s", ErrEmptyCollection, path)
```

#### New Code

```go
// errors.go
var ErrMissingRequiredField = errors.New("missing required field")

// parser.go
import apierrors "github.com/weiqigod/curlew/internal/errors"

// ParseFile wraps errors in Structured for file/line context
return nil, &apierrors.Structured{
    Category: apierrors.CategoryParse,
    FilePath: path,
    Message:  fmt.Sprintf("file not found: %s", path),
    Inner:    ErrFileNotFound,
}

return nil, &apierrors.Structured{
    Category: apierrors.CategoryParse,
    FilePath: path,
    Line:     extractYAMLLine(err),
    Message:  fmt.Sprintf("invalid YAML syntax: %s", err),
    Inner:    ErrInvalidYAML,
}

// New validation function
func validateRequests(path string, items []RequestItem) error {
    for _, item := range items {
        if item.URL == "" {
            return &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: path,
                Message:  fmt.Sprintf("request %q is missing required field 'url'", item.Name),
                Hint:     "Every request must specify a url field",
                Inner:    ErrMissingRequiredField,
            }
        }
    }
    return nil
}

// extractYAMLLine parses line number from yaml.v3 error string
func extractYAMLLine(err error) int {
    // yaml.v3 format: "yaml: line N: ..."
    // Use regexp or string parsing
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_StructuredErrors(t *testing.T) {
    tests := []struct {
        name         string
        input        string // file content or path
        wantSentinel error
        wantFilePath bool
        wantLine     bool
    }{
        {"invalid yaml returns structured error with file path", ":\ninvalid", ErrInvalidYAML, true, true},
        {"invalid yaml includes line info", "valid:\n  -\n    :\nbad", ErrInvalidYAML, true, true},
        {"file not found returns structured error", "", ErrFileNotFound, true, false},
        {"empty collection returns structured error", "name: test\nrequests: []", ErrEmptyCollection, true, false},
    }
    ...
}

func TestParseFile_MissingRequiredFields(t *testing.T) {
    tests := []struct {
        name        string
        input       string
        wantErr     error
        wantMessage string
    }{
        {"missing url returns structured error", "name: test\nrequests:\n  - name: no-url\n    method: GET", ErrMissingRequiredField, "missing required field 'url'"},
        {"missing url names the request", "name: test\nrequests:\n  - name: my-req\n    method: GET", ErrMissingRequiredField, "my-req"},
    }
    ...
}
```

#### Impact on Existing Tests
- Existing tests using `errors.Is(err, ErrInvalidYAML)` etc. will **continue to work** because `Structured.Unwrap()` returns the sentinel
- No tests should break — the `errors.Is` chain is preserved

---

### Step 3: Enhance httpexec to classify network errors

**Rationale:** Depends on `internal/errors` from Step 1. Modifying httpexec is isolated — only the runner calls it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor.go` | modify | Use `ClassifyNetworkError` for error wrapping |
| `internal/httpexec/executor_test.go` | modify | Add classification tests, update `TestExecute_network_error_preserves_inner_error` |

#### Current Code

```go
// executor.go
return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
```

#### New Code

```go
// executor.go
import apierrors "github.com/weiqigod/curlew/internal/errors"

// In Execute, when http.Do fails:
netErr := apierrors.ClassifyNetworkError(err)
netErr.Inner = fmt.Errorf("%w: %w", ErrNetwork, netErr.Inner)
return nil, netErr
```

Note: We wrap `ErrNetwork` into the inner chain so `errors.Is(err, ErrNetwork)` still works.

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_NetworkErrorClassification(t *testing.T) {
    tests := []struct {
        name     string
        setup    func() (*parser.Request, context.Context)
        wantKind apierrors.NetworkErrorKind
        wantHint string
    }{
        {"connection refused classifies correctly", connRefusedSetup, apierrors.NetworkConnectionRefused, "server is running"},
        {"connection refused suggests server check", connRefusedSetup, apierrors.NetworkConnectionRefused, "server is running"},
        {"dns failure classifies correctly", dnsFailSetup, apierrors.NetworkDNS, "hostname"},
        {"dns failure distinguishes from connection refused", dnsFailSetup, apierrors.NetworkDNS, ""},
        {"timeout classifies correctly", timeoutSetup, apierrors.NetworkTimeout, "timeout"},
        {"timeout includes duration hint", timeoutSetup, apierrors.NetworkTimeout, "increasing"},
        {"classified errors preserve ErrNetwork", connRefusedSetup, apierrors.NetworkConnectionRefused, ""},
    }
    ...
}
```

#### Impact on Existing Tests
- **`TestExecute_network_error_preserves_inner_error`** — will break because it checks for `Unwrap() []error` (join error). Needs updating to use `errors.As(err, &netErr)` pattern instead.
- **`TestExecute_network_error_on_connection_refused`** — still passes because `errors.Is(err, ErrNetwork)` is preserved in the chain.

---

### Step 4: Update output package for consistent error formatting

**Rationale:** Depends on error types from Step 1. Output is a leaf package — nothing depends on its format programmatically.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `PrintStructuredError` and `PrintRequestError` |
| `internal/output/terminal_test.go` | modify | Add tests for new formatting functions |

#### Current Code

```go
func PrintError(w io.Writer, msg string) {
    fmt.Fprintf(w, "Error: %s\n", msg)
}
```

#### New Code

```go
import apierrors "github.com/weiqigod/curlew/internal/errors"

// PrintStructuredError writes a formatted error using the [ERROR] format.
func PrintStructuredError(w io.Writer, err error) {
    fmt.Fprintln(w, apierrors.Format(err))
}

// PrintRequestError writes an error for a named request.
// Format: [ERROR] <request-name> — <classified message>
func PrintRequestError(w io.Writer, name string, err error) {
    var netErr *apierrors.NetworkError
    if errors.As(err, &netErr) {
        fmt.Fprintf(w, "[ERROR] %s — %s\n", name, netErr.Message)
        if netErr.Hint != "" {
            fmt.Fprintf(w, "  Hint: %s\n", netErr.Hint)
        }
        return
    }
    fmt.Fprintf(w, "[ERROR] %s — %s\n", name, err)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrintStructuredError(t *testing.T) {
    tests := []struct {
        name string
        err  error
        want string
    }{
        {"structured with file and line", &apierrors.Structured{FilePath: "f.yaml", Line: 3, Message: "bad"}, "[ERROR] f.yaml:3 — bad\n"},
        {"plain error", errors.New("boom"), "[ERROR] boom\n"},
    }
    ...
}

func TestPrintRequestError(t *testing.T) {
    tests := []struct {
        name    string
        reqName string
        err     error
        want    string
    }{
        {"network error with hint", "GET /users", &apierrors.NetworkError{Message: "DNS failed", Hint: "check host"}, "[ERROR] GET /users — DNS failed\n  Hint: check host\n"},
        {"plain error", "GET /users", errors.New("boom"), "[ERROR] GET /users — boom\n"},
    }
    ...
}
```

#### Impact on Existing Tests
- **`TestPrintError`** — NOT affected, `PrintError` is preserved unchanged
- New functions are additive

---

### Step 5: Wire everything together in main.go

**Rationale:** Final integration. Depends on all previous steps.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Use `PrintStructuredError` and `PrintRequestError` |
| `cmd/curlew/main_test.go` | modify | Update integration tests for new error format |
| `cmd/curlew/run_test.go` | modify | Update expected error output if needed |

#### Current Code

```go
// main.go - parse error
output.PrintError(os.Stderr, err.Error())

// main.go - request error
output.PrintError(os.Stderr, fmt.Sprintf("%s: %v", r.Name, r.Err))
```

#### New Code

```go
// main.go - parse error
output.PrintStructuredError(os.Stderr, err)

// main.go - request error
output.PrintRequestError(os.Stderr, r.Name, r.Err)
```

#### Tests to Write FIRST (RED phase)

```go
// Integration tests verifying end-to-end error format
func TestCLIIntegration_ErrorFormat(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        fixture string // temp file content
        wantErr string
    }{
        {"parse error has ERROR prefix", []string{"run", "bad.yaml"}, ":\ninvalid", "[ERROR]"},
        {"parse error includes file path", []string{"run", "bad.yaml"}, ":\ninvalid", "bad.yaml"},
        {"missing file has ERROR prefix", []string{"run", "nonexistent.yaml"}, "", "[ERROR]"},
        {"network error has ERROR prefix", []string{"run", "conn.yaml"}, connRefusedYAML, "[ERROR]"},
        {"network error suggests server check", []string{"run", "conn.yaml"}, connRefusedYAML, "server is running"},
    }
    ...
}
```

#### Impact on Existing Tests
- Integration tests checking `wantErr` for substrings like `"not found"`, `"invalid YAML"` — may need updating if the new format changes the error text. Since `strings.Contains` is used and the substrings are still present, most should pass.

---

### Step 6: Update smoke test

**Rationale:** Last step — validates end-to-end after everything is wired.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add structured error format checks |

#### New Code

```bash
echo "--- Structured error format ---"
OUTPUT=$(./curlew run nonexistent.yaml 2>&1 || true)
echo "$OUTPUT" | grep -q "\[ERROR\]" && echo "PASS: [ERROR] prefix present" || fail "Missing [ERROR] prefix"
echo "$OUTPUT" | grep -q "nonexistent.yaml" && echo "PASS: file path in error" || fail "Missing file path in error"
```

#### Impact on Existing Tests
- Existing smoke test assertions for error scenarios may need updating if they check for `"Error:"` prefix

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/httpexec/executor_test.go` | `TestExecute_network_error_preserves_inner_error` | breaks | Update to use `errors.As` for `NetworkError` |
| `internal/httpexec/executor_test.go` | `TestExecute_network_error_on_connection_refused` | none | `errors.Is(err, ErrNetwork)` still works |
| `internal/parser/parser_test.go` | All existing tests | none | `errors.Is` chain preserved via `Structured.Unwrap()` |
| `internal/output/terminal_test.go` | `TestPrintError` | none | `PrintError` preserved unchanged |
| `cmd/curlew/main_test.go` | `TestCLIIntegration` | may need update | Error format changes from `Error:` to `[ERROR]` |
| `cmd/curlew/run_test.go` | Various | may need update | Check for new error format |

## Risks and Edge Cases

- **Risk:** yaml.v3 line number extraction relies on parsing error message strings → **Mitigation:** Use regex with fallback to `Line=0` if parsing fails
- **Risk:** TLS error classification varies across platforms (macOS vs Linux) → **Mitigation:** Check both `*tls.CertificateVerificationError` and `x509` errors via `errors.As`; fallback to string matching for "certificate" or "tls"
- **Risk:** DNS errors wrapped in `*url.Error` may not match `*net.DNSError` directly → **Mitigation:** `errors.As` traverses the full error chain
- **Edge case:** Request with empty URL currently fails at execution time (network error) — new validation moves failure to parse time → **Handling:** This is correct behavior; catch errors early. Exit code changes from 4 to 3 for this case.
- **Edge case:** Network errors don't have file:line — they have request name → **Handling:** Use `[ERROR] <request-name> — message` for runtime errors, `[ERROR] <file>:<line> — message` for parse errors
- **Edge case:** Timeout classification without `--timeout` flag → **Handling:** Classify `context.DeadlineExceeded` as timeout; full `--timeout` support comes in a later task

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Parse error with file and line
echo ":\ninvalid" > /tmp/bad.yaml
./curlew run /tmp/bad.yaml
# Expected: [ERROR] /tmp/bad.yaml:1 — invalid YAML syntax: ...

# Missing required field
echo -e "name: test\nrequests:\n  - name: no-url\n    method: GET" > /tmp/nourl.yaml
./curlew run /tmp/nourl.yaml
# Expected: [ERROR] /tmp/nourl.yaml — request "no-url" is missing required field 'url'

# Connection refused
echo -e "name: test\nrequests:\n  - name: fail\n    method: GET\n    url: http://127.0.0.1:1/test" > /tmp/connrefused.yaml
./curlew run /tmp/connrefused.yaml
# Expected: [ERROR] fail — Connection refused at 127.0.0.1:1
#           Hint: Check that the server is running and listening on this port

# DNS failure
echo -e "name: test\nrequests:\n  - name: dns-fail\n    method: GET\n    url: http://nonexistent.invalid/test" > /tmp/dnsfail.yaml
./curlew run /tmp/dnsfail.yaml
# Expected: [ERROR] dns-fail — DNS resolution failed for nonexistent.invalid
#           Hint: Check that the hostname is correct and DNS is configured

# Missing file
./curlew run nonexistent.yaml
# Expected: [ERROR] nonexistent.yaml — file not found: nonexistent.yaml
```
