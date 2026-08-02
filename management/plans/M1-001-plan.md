# Implementation Plan: M1-001

## Overview

Deliver the first runnable vertical slice: parse a YAML collection file with one GET request, execute it over HTTP, and print the request name, status code, and duration to the terminal. Includes proper exit codes, error handling, CLI `run` command, and a sample collection file.

## Task Details
- **ID:** M1-001
- **Title:** Run a single GET request from a collection file
- **Phase:** M1: Core CLI
- **Priority:** 1
- **Complexity:** high

## Dependencies

None — this is the root task.

## Implementation Steps

### Step 1: Define collection types and parse YAML (`internal/parser/`)
**Rationale:** Types are the foundation everything else depends on. No blast radius — nothing exists yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | create | Collection, RequestItem, Request types |
| `internal/parser/parser.go` | create | ParseFile function |
| `internal/parser/parser_test.go` | create | Table-driven tests for parsing |
| `internal/parser/errors.go` | create | Sentinel errors |

#### New Code

```go
// internal/parser/errors.go
package parser

import "errors"

var (
    ErrFileNotFound    = errors.New("collection file not found")
    ErrInvalidYAML     = errors.New("invalid YAML syntax")
    ErrEmptyCollection = errors.New("collection has no name")
)
```

```go
// internal/parser/collection.go
package parser

// Collection represents a parsed collection file.
type Collection struct {
    Name        string        `yaml:"name"`
    Description string        `yaml:"description,omitempty"`
    Requests    []RequestItem `yaml:"requests"`
}

// RequestItem is a single test entry in a collection.
type RequestItem struct {
    Name    string  `yaml:"name"`
    Request Request `yaml:"request"`
}

// Request defines the HTTP request to execute.
type Request struct {
    Method  string            `yaml:"method"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers,omitempty"`
}
```

```go
// internal/parser/parser.go
package parser

import (
    "errors"
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

// ParseFile reads and parses a collection YAML file.
func ParseFile(path string) (*Collection, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, fmt.Errorf("%w: %s", ErrFileNotFound, path)
        }
        return nil, fmt.Errorf("reading collection file: %w", err)
    }

    var col Collection
    if err := yaml.Unmarshal(data, &col); err != nil {
        return nil, fmt.Errorf("%w: %v", ErrInvalidYAML, err)
    }

    if col.Name == "" {
        return nil, ErrEmptyCollection
    }

    return &col, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile(t *testing.T) {
    tests := []struct {
        name    string
        file    string // path to testdata file
        wantErr error
        wantCol *Collection
    }{
        {
            name: "valid minimal collection",
            file: "testdata/minimal.yaml",
            wantCol: &Collection{
                Name: "Minimal Test",
                Requests: []RequestItem{
                    {
                        Name: "Get Example",
                        Request: Request{Method: "GET", URL: "https://example.com"},
                    },
                },
            },
        },
        {
            name:    "file not found",
            file:    "testdata/nonexistent.yaml",
            wantErr: ErrFileNotFound,
        },
        {
            name:    "invalid YAML",
            file:    "testdata/invalid.yaml",
            wantErr: ErrInvalidYAML,
        },
        {
            name:    "empty collection name",
            file:    "testdata/no_name.yaml",
            wantErr: ErrEmptyCollection,
        },
        {
            name: "collection with headers",
            file: "testdata/with_headers.yaml",
            wantCol: &Collection{
                Name: "Headers Test",
                Requests: []RequestItem{
                    {
                        Name: "Get With Headers",
                        Request: Request{
                            Method:  "GET",
                            URL:     "https://example.com",
                            Headers: map[string]string{"Accept": "application/json"},
                        },
                    },
                },
            },
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            col, err := ParseFile(tt.file)
            if tt.wantErr != nil {
                if !errors.Is(err, tt.wantErr) {
                    t.Fatalf("got error %v, want %v", err, tt.wantErr)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            // Compare relevant fields
        })
    }
}
```

#### Test Fixtures

| File | Content |
|------|---------|
| `internal/parser/testdata/minimal.yaml` | Minimal valid collection with one GET |
| `internal/parser/testdata/invalid.yaml` | Broken YAML syntax |
| `internal/parser/testdata/no_name.yaml` | Valid YAML but missing `name` field |
| `internal/parser/testdata/with_headers.yaml` | Collection with request headers |

#### Impact on Existing Tests
- No existing tests — this is the first code in the package.

#### External Dependency
- `gopkg.in/yaml.v3` — required for YAML parsing (standard choice for Go, spec acknowledges "YAML with one library")

---

### Step 2: HTTP executor (`internal/http/`)
**Rationale:** Depends on parser types for request input. Small, focused package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/http/executor.go` | create | Execute function, Result type |
| `internal/http/executor_test.go` | create | Tests using httptest server |
| `internal/http/errors.go` | create | Sentinel errors |

#### New Code

```go
// internal/http/errors.go
package http

import "errors"

var ErrNetwork = errors.New("network error")
```

```go
// internal/http/executor.go
package http

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/weiqigod/curlew/internal/parser"
)

// Result holds the outcome of an executed HTTP request.
type Result struct {
    StatusCode int
    Duration   time.Duration
}

// Execute sends an HTTP request and returns the result.
func Execute(ctx context.Context, req *parser.Request) (*Result, error) {
    httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
    if err != nil {
        return nil, fmt.Errorf("building request: %w", err)
    }

    for k, v := range req.Headers {
        httpReq.Header.Set(k, v)
    }

    start := time.Now()
    resp, err := http.DefaultClient.Do(httpReq)
    duration := time.Since(start)
    if err != nil {
        return nil, fmt.Errorf("%w: %v", ErrNetwork, err)
    }
    defer resp.Body.Close()

    return &Result{
        StatusCode: resp.StatusCode,
        Duration:   duration,
    }, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecute(t *testing.T) {
    tests := []struct {
        name       string
        handler    http.HandlerFunc
        req        *parser.Request
        wantStatus int
        wantErr    error
    }{
        {
            name:       "successful GET returns 200",
            handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) },
            req:        &parser.Request{Method: "GET", URL: ""},  // URL set to test server
            wantStatus: 200,
        },
        {
            name:       "server returns 404",
            handler:    func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) },
            req:        &parser.Request{Method: "GET", URL: ""},
            wantStatus: 404,
        },
        {
            name: "request headers are sent",
            handler: func(w http.ResponseWriter, r *http.Request) {
                if r.Header.Get("Accept") != "application/json" {
                    w.WriteHeader(400)
                    return
                }
                w.WriteHeader(200)
            },
            req:        &parser.Request{Method: "GET", URL: "", Headers: map[string]string{"Accept": "application/json"}},
            wantStatus: 200,
        },
        {
            name:    "network error on unreachable host",
            req:     &parser.Request{Method: "GET", URL: "http://192.0.2.1:1/unreachable"},
            wantErr: ErrNetwork,
        },
        {
            name:    "context cancellation",
            // Use already-cancelled context
            wantErr: ErrNetwork,
        },
    }
    // Each test starts an httptest.NewServer (except error cases) and sets req.URL
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 3: Terminal output formatter (`internal/output/`)
**Rationale:** Depends on http.Result type. Thin formatting layer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | create | PrintResult, PrintError functions |
| `internal/output/terminal_test.go` | create | Tests capturing stdout output |

#### New Code

```go
// internal/output/terminal.go
package output

import (
    "fmt"
    "io"

    httpexec "github.com/weiqigod/curlew/internal/http"
)

// PrintResult writes a single request result line to w.
func PrintResult(w io.Writer, name string, result *httpexec.Result) {
    fmt.Fprintf(w, "  %s  %d  %dms\n", name, result.StatusCode, result.Duration.Milliseconds())
}

// PrintCollectionHeader writes the collection name header.
func PrintCollectionHeader(w io.Writer, name string) {
    fmt.Fprintf(w, "Collection: %s\n", name)
}

// PrintSummary writes the execution summary.
func PrintSummary(w io.Writer, total, passed, failed int) {
    fmt.Fprintf(w, "\n%d request(s): %d passed, %d failed\n", total, passed, failed)
}

// PrintError writes an error message to w.
func PrintError(w io.Writer, msg string) {
    fmt.Fprintf(w, "Error: %s\n", msg)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrintResult(t *testing.T) {
    tests := []struct {
        name   string
        rName  string
        result *httpexec.Result
        want   string // expected substring in output
    }{
        {
            name:   "shows name, status, and duration",
            rName:  "Get Users",
            result: &httpexec.Result{StatusCode: 200, Duration: 150 * time.Millisecond},
            want:   "Get Users  200  150ms",
        },
    }
}

func TestPrintCollectionHeader(t *testing.T) { /* ... */ }
func TestPrintSummary(t *testing.T) { /* ... */ }
func TestPrintError(t *testing.T) { /* ... */ }
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 4: CLI wiring — `run` command (`cmd/curlew/main.go`)
**Rationale:** Depends on all three packages. This is the integration point.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `run` command routing, wire parser→http→output |

#### Current Code

```go
package main

import (
    "fmt"
    "os"
)

const version = "0.1.0-dev"

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--version" {
        fmt.Printf("curlew %s\n", version)
        return
    }

    fmt.Println("curlew — a file-based API testing tool")
    // ... help text
}
```

#### New Code

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "os"

    httpexec "github.com/weiqigod/curlew/internal/http"
    "github.com/weiqigod/curlew/internal/output"
    "github.com/weiqigod/curlew/internal/parser"
)

const version = "0.1.0-dev"

func main() {
    os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
    if len(args) == 0 {
        printHelp()
        return 0
    }

    switch args[0] {
    case "--version":
        fmt.Printf("curlew %s\n", version)
        return 0
    case "--help", "-h":
        printHelp()
        return 0
    case "run":
        return runCmd(args[1:])
    default:
        fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
        printHelp()
        return 1
    }
}

func runCmd(args []string) int {
    if len(args) == 0 {
        fmt.Fprintln(os.Stderr, "Usage: curlew run <collection-file>")
        return 1
    }

    col, err := parser.ParseFile(args[0])
    if err != nil {
        output.PrintError(os.Stderr, err.Error())
        if errors.Is(err, parser.ErrFileNotFound) || errors.Is(err, parser.ErrInvalidYAML) || errors.Is(err, parser.ErrEmptyCollection) {
            return 3
        }
        return 3
    }

    ctx := context.Background()
    output.PrintCollectionHeader(os.Stdout, col.Name)

    passed, failed := 0, 0
    for _, item := range col.Requests {
        result, err := httpexec.Execute(ctx, &item.Request)
        if err != nil {
            output.PrintError(os.Stderr, fmt.Sprintf("%s: %v", item.Name, err))
            if errors.Is(err, httpexec.ErrNetwork) {
                failed++
                continue
            }
            failed++
            continue
        }
        output.PrintResult(os.Stdout, item.Name, result)
        passed++
    }

    output.PrintSummary(os.Stdout, passed+failed, passed, failed)

    if failed > 0 {
        return 4
    }
    return 0
}

func printHelp() {
    fmt.Println("curlew — a file-based API testing tool")
    fmt.Println()
    fmt.Printf("Version: %s\n", version)
    fmt.Println()
    fmt.Println("Usage:")
    fmt.Println("  curlew <command> [arguments]")
    fmt.Println()
    fmt.Println("Commands:")
    fmt.Println("  run <file>   Execute requests in a collection file")
    fmt.Println()
    fmt.Println("Options:")
    fmt.Println("  --version    Show version information")
    fmt.Println("  --help       Show this help message")
}
```

#### Impact on Existing Tests
- No existing test files. The current `main.go` will be fully replaced.

---

### Step 5: Integration tests
**Rationale:** After all packages work individually, verify the full pipeline by building and invoking the binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main_test.go` | create | Integration tests building and running the binary |
| `cmd/curlew/testdata/minimal.yaml` | create | Valid collection fixture |
| `cmd/curlew/testdata/invalid.yaml` | create | Broken YAML fixture |
| `cmd/curlew/testdata/no_name.yaml` | create | Missing name fixture |

#### Tests to Write FIRST (RED phase)

```go
func TestCLIIntegration(t *testing.T) {
    // Build binary once
    binary := buildBinary(t)

    tests := []struct {
        name     string
        args     []string
        wantExit int
        wantOut  string // substring in stdout
        wantErr  string // substring in stderr
    }{
        {
            name:     "no args shows help",
            args:     []string{},
            wantExit: 0,
            wantOut:  "Usage:",
        },
        {
            name:     "help flag",
            args:     []string{"--help"},
            wantExit: 0,
            wantOut:  "Commands:",
        },
        {
            name:     "version flag",
            args:     []string{"--version"},
            wantExit: 0,
            wantOut:  "curlew",
        },
        {
            name:     "run without file shows usage",
            args:     []string{"run"},
            wantExit: 1,
            wantErr:  "Usage:",
        },
        {
            name:     "run with missing file",
            args:     []string{"run", "testdata/nonexistent.yaml"},
            wantExit: 3,
            wantErr:  "not found",
        },
        {
            name:     "run with invalid YAML",
            args:     []string{"run", "testdata/invalid.yaml"},
            wantExit: 3,
            wantErr:  "invalid YAML",
        },
        {
            name:     "unknown command",
            args:     []string{"bogus"},
            wantExit: 1,
            wantErr:  "Unknown command",
        },
    }
}
```

Note: A test against a real HTTP endpoint (e.g., httptest server started in the test) will verify the successful run path with exit code 0.

---

### Step 6: Sample file and smoke test
**Rationale:** Final step — delivers observable output. Depends on everything above working.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `sample/hello.yaml` | create | Working example collection |
| `smoke/run.sh` | modify | Add `curlew run` smoke scenario |

#### Sample File

```yaml
name: Hello API
description: A simple example collection

requests:
  - name: Get httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"
```

#### Updated Smoke Test

Add after existing version check:
```bash
echo "--- Running sample collection ---"
./curlew run sample/hello.yaml
echo
```

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| — | — | No existing tests | All tests are new |

## Risks and Edge Cases

- **Risk:** `gopkg.in/yaml.v3` dependency must be added → **Mitigation:** `go get gopkg.in/yaml.v3` before building; it's the standard Go YAML library.
- **Risk:** httpbin.org may be unreachable in some environments → **Mitigation:** Integration tests use `httptest.NewServer` locally; only `smoke/run.sh` hits the real endpoint. Smoke is manual and advisory.
- **Edge case:** Empty requests array in collection → **Handling:** Treat as success (0 requests, 0 failures, exit 0). Not an error — the collection is valid, just has nothing to do.
- **Edge case:** Request with no method specified → **Handling:** Go's `http.NewRequest` defaults to GET when method is empty. Accept this for now; validation of required fields is a future concern.
- **Edge case:** URL with no scheme → **Handling:** Let Go's http client return an error, which wraps as ErrNetwork.
- **Risk:** Network errors should return exit 4 but assertion failures (M1-004+) should return exit 1 → **Mitigation:** For M1-001 (no assertions yet), any request failure during execution is a network error (exit 4). The exit code logic will be refined when assertions are added.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
./curlew run sample/hello.yaml
# Expected: "Collection: Hello API" header, "Get httpbin  200  <N>ms" line, summary
```

```bash
./curlew run nonexistent.yaml
echo $?
# Expected: error message, exit code 3
```

```bash
./curlew run
echo $?
# Expected: usage message, exit code 1
```

```bash
./curlew --version
# Expected: "curlew 0.1.0-dev"
```

```bash
./curlew --help
# Expected: help text with "run" command listed
```
