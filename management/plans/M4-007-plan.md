# Implementation Plan: M4-007

## Overview

Add a `pr-check` subcommand to the Go CLI that reads a local test results file, POSTs it to the backend results API, then POSTs a PR check status payload. Includes retry, dry-run, help, and error handling. All logic lives in a new `internal/prcheck` package with the thin CLI wiring in `cmd/apitest/main.go`.

## Task Details
- **ID:** M4-007
- **Title:** CLI: pr-check subcommand posting status to backend
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-004 | Backend: test results ingestion API | done |

## Implementation Steps

### Step 1: Create `internal/prcheck` Package — Types and Config

**Rationale:** Define data types and config parsing first. Zero blast radius — no existing code is touched. All subsequent steps depend on these types.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/prcheck.go` | create | Config, request/response types, sentinel errors |

#### New Code

```go
package prcheck

import (
	"errors"
	"fmt"
	"os"
)

// Sentinel errors for well-known failure modes.
var (
	ErrBackendURLMissing = errors.New("backend URL not configured")
	ErrUnauthorized      = errors.New("unauthorized: refresh APITEST_BACKEND_TOKEN")
	ErrNetworkFailure    = errors.New("network error: backend unreachable after retries")
)

// Config holds the pr-check configuration derived from flags and env vars.
type Config struct {
	BackendURL   string // APITEST_BACKEND_URL
	BackendToken string // APITEST_BACKEND_TOKEN
	Org          string // --org
	PR           int    // --pr
	Repo         string // --repo
	ResultsFile  string // --results
	DryRun       bool   // --dry-run
}

// ConfigFromEnv populates Config fields from environment variables.
// Flags must be set separately by the caller.
func ConfigFromEnv() Config {
	return Config{
		BackendURL:   os.Getenv("APITEST_BACKEND_URL"),
		BackendToken: os.Getenv("APITEST_BACKEND_TOKEN"),
	}
}

// Validate checks required fields. Returns an error if any are missing.
func (c *Config) Validate() error {
	if c.BackendURL == "" {
		return ErrBackendURLMissing
	}
	if c.BackendToken == "" {
		return fmt.Errorf("APITEST_BACKEND_TOKEN is required")
	}
	if c.Org == "" {
		return fmt.Errorf("--org is required")
	}
	if c.PR <= 0 {
		return fmt.Errorf("--pr must be a positive integer")
	}
	if c.Repo == "" {
		return fmt.Errorf("--repo is required")
	}
	if c.ResultsFile == "" {
		return fmt.Errorf("--results is required")
	}
	return nil
}

// ResultsPayload is the JSON body POSTed to /api/v1/organizations/{org}/results.
// Matches the backend UploadResultRequest (snake_case JSON).
type ResultsPayload struct {
	CollectionName string        `json:"collection_name"`
	RunAt          string        `json:"run_at"`
	DurationMs     int64         `json:"duration_ms"`
	PassCount      int           `json:"pass_count"`
	FailCount      int           `json:"fail_count"`
	SkippedCount   int           `json:"skipped_count"`
	TriggeredBy    string        `json:"triggered_by"`
	GitSha         string        `json:"git_sha,omitempty"`
	Items          []ResultItem  `json:"items"`
}

// ResultItem is a single test case in the results payload.
type ResultItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Message    *string `json:"message"`
}

// IngestResponse is the response from POST /results (202 Accepted).
type IngestResponse struct {
	ResultID string `json:"result_id"`
	Status   string `json:"status"`
}

// PrCheckPayload is the body POSTed to /api/v1/organizations/{org}/pr-checks.
type PrCheckPayload struct {
	Repo     string `json:"repo"`
	PR       int    `json:"pr"`
	State    string `json:"state"` // "success" or "failure"
	ResultID string `json:"result_id"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"valid_config", Config{BackendURL: "http://localhost", BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"}, ""},
		{"missing_backend_url", Config{BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"}, "backend URL not configured"},
		{"missing_backend_token", Config{BackendURL: "http://x", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"}, "APITEST_BACKEND_TOKEN"},
		{"missing_org", Config{BackendURL: "http://x", BackendToken: "tok", PR: 42, Repo: "acme/api", ResultsFile: "f.json"}, "--org"},
		{"zero_pr", Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 0, Repo: "acme/api", ResultsFile: "f.json"}, "--pr"},
		{"missing_repo", Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 42, ResultsFile: "f.json"}, "--repo"},
		{"missing_results", Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api"}, "--results"},
	}
	// ...
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name           string
		envURL         string
		envToken       string
		wantURL        string
		wantToken      string
	}{
		{"both_set", "http://back", "tok", "http://back", "tok"},
		{"empty_env", "", "", "", ""},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new package).

---

### Step 2: Create `internal/prcheck` — File Loading and Result Detection

**Rationale:** Parsing the results file and detecting pass/fail is the next building block, still isolated from HTTP.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/prcheck.go` | modify | Add LoadResultsFile function |

#### New Code

```go
// LoadResultsFile reads and parses a JSON results file.
// Returns the payload and whether all tests passed (fail_count == 0).
func LoadResultsFile(path string) (*ResultsPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading results file: %w", err)
	}
	var payload ResultsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parsing results JSON: %w", err)
	}
	if payload.CollectionName == "" {
		return nil, fmt.Errorf("results file missing collection_name")
	}
	return &payload, nil
}

// HasFailures returns true if the payload contains any failing tests.
func (p *ResultsPayload) HasFailures() bool {
	return p.FailCount > 0
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestLoadResultsFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
		wantPass int
		wantFail int
	}{
		{"valid_all_pass", validJSON, "", 3, 0},
		{"valid_with_failures", failingJSON, "", 2, 1},
		{"invalid_json", "{bad", "parsing results JSON", 0, 0},
		{"missing_collection_name", noNameJSON, "missing collection_name", 0, 0},
		{"file_not_found", "", "reading results file", 0, 0},
	}
	// ...
}

func TestResultsPayload_HasFailures(t *testing.T) {
	tests := []struct {
		name      string
		failCount int
		want      bool
	}{
		{"no_failures", 0, false},
		{"has_failures", 1, true},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 3: Create `internal/prcheck` — HTTP Client with Retry

**Rationale:** The HTTP client encapsulates backend communication with retry logic. Uses `net/http` standard library and a simple retry loop (2 retries, 200ms backoff) — not the existing `internal/retry` package which is for collection request retries with complex condition matching.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/client.go` | create | HTTP client with retry, upload results, post pr-check |
| `internal/prcheck/client_test.go` | create | Tests using httptest.Server |

#### New Code

```go
package prcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	maxRetries    = 2
	retryInterval = 200 * time.Millisecond
)

// Client posts results and PR check status to the backend.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client // nil = http.DefaultClient
}

// UploadResults POSTs the results payload and returns the result ID.
func (c *Client) UploadResults(ctx context.Context, org string, payload *ResultsPayload) (*IngestResponse, error) {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/results", c.BaseURL, org)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling results: %w", err)
	}
	respBody, err := c.doWithRetry(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	var resp IngestResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing ingest response: %w", err)
	}
	return &resp, nil
}

// PostPrCheck POSTs the PR check status payload.
func (c *Client) PostPrCheck(ctx context.Context, org string, payload *PrCheckPayload) error {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/pr-checks", c.BaseURL, org)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling pr-check: %w", err)
	}
	_, err = c.doWithRetry(ctx, "POST", url, body)
	return err
}

// doWithRetry sends an HTTP request with up to maxRetries retries on
// connection errors. Returns ErrUnauthorized on 401, ErrNetworkFailure
// after all retries are exhausted.
func (c *Client) doWithRetry(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryInterval):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue // retry on connection error
		}
		defer resp.Body.Close()
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == 401 {
			return nil, ErrUnauthorized
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}
		return nil, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil, fmt.Errorf("%w: %v", ErrNetworkFailure, lastErr)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestClient_UploadResults(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		respBody   string
		wantErr    string
		wantID     string
	}{
		{"success_202", 202, `{"result_id":"res_abc","status":"accepted"}`, "", "res_abc"},
		{"unauthorized_401", 401, `{"error":"unauthorized"}`, "unauthorized", ""},
		{"server_error_500", 500, `{"error":"internal"}`, "HTTP 500", ""},
	}
	// each test creates an httptest.Server, sets the response, and calls UploadResults
}

func TestClient_PostPrCheck(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{"success_200", 200, ""},
		{"unauthorized_401", 401, "unauthorized"},
	}
	// ...
}

func TestClient_RetryOnConnectionRefused(t *testing.T) {
	// Start a server, immediately close it, then call UploadResults.
	// Verify ErrNetworkFailure is returned and exactly 3 attempts were made.
}

func TestClient_RetryBackoff(t *testing.T) {
	// Track timestamps of requests hitting the server to verify ~200ms gap.
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 4: Create `internal/prcheck` — Run Orchestrator

**Rationale:** Ties config, file loading, and client together into a single `Run` function. Still isolated from the CLI entry point.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/run.go` | create | Top-level Run function |
| `internal/prcheck/run_test.go` | create | Integration-style tests with httptest |

#### New Code

```go
package prcheck

import (
	"context"
	"fmt"
	"io"
)

// RunResult holds the outcome of a pr-check run.
type RunResult struct {
	ResultID string
	State    string // "success" or "failure"
	Pass     int
	Fail     int
}

// Run orchestrates the full pr-check flow:
// 1. Validate config
// 2. Load results file
// 3. Upload results to backend (unless dry-run)
// 4. Post PR check status (unless dry-run)
// Returns a RunResult on success. On dry-run, prints the payloads to w.
func Run(ctx context.Context, cfg Config, w io.Writer) (*RunResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	payload, err := LoadResultsFile(cfg.ResultsFile)
	if err != nil {
		return nil, err
	}

	state := "success"
	if payload.HasFailures() {
		state = "failure"
	}

	if cfg.DryRun {
		// Print what would be sent
		printDryRun(w, cfg, payload, state)
		return &RunResult{State: state, Pass: payload.PassCount, Fail: payload.FailCount}, nil
	}

	client := &Client{
		BaseURL: cfg.BackendURL,
		Token:   cfg.BackendToken,
	}

	resp, err := client.UploadResults(ctx, cfg.Org, payload)
	if err != nil {
		return nil, err
	}

	prPayload := &PrCheckPayload{
		Repo:     cfg.Repo,
		PR:       cfg.PR,
		State:    state,
		ResultID: resp.ResultID,
	}
	if err := client.PostPrCheck(ctx, cfg.Org, prPayload); err != nil {
		return nil, err
	}

	return &RunResult{
		ResultID: resp.ResultID,
		State:    state,
		Pass:     payload.PassCount,
		Fail:     payload.FailCount,
	}, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_Success(t *testing.T) {
	// httptest server that accepts both POST /results and POST /pr-checks
	// Verify returned RunResult has correct fields
}

func TestRun_DryRun_NoHTTP(t *testing.T) {
	// No server running. dry-run should not dial out.
	// Capture stdout and verify JSON payloads are printed.
}

func TestRun_FailingTests_StateFailure(t *testing.T) {
	// Results file with fail_count > 0
	// Verify state="failure" in the pr-check payload and RunResult
}

func TestRun_MissingConfig_Error(t *testing.T) {
	// Empty config → error before any HTTP
}

func TestRun_Unauthorized_Error(t *testing.T) {
	// Server returns 401 → ErrUnauthorized
}

func TestRun_ConnectionRefused_Error(t *testing.T) {
	// Bad URL → ErrNetworkFailure after retries
}

func TestRun_BadResultsFile_Error(t *testing.T) {
	// Non-existent file → error
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 5: Wire `pr-check` Subcommand into CLI

**Rationale:** Now that the `internal/prcheck` package is complete and tested, wire it into `cmd/apitest/main.go` as a new subcommand. This touches existing code but the change is small and additive.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Add `case "pr-check"` to `run()` switch, add `prCheckCmd` function, update `printHelp` |

#### Current Code

```go
// In run() switch:
case "import":
    return importCmd(args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
```

#### New Code

```go
// In run() switch:
case "pr-check":
    return prCheckCmd(args[1:])
case "import":
    return importCmd(args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
```

New `prCheckCmd` function:

```go
func prCheckCmd(args []string) int {
	cfg, showHelp, err := parsePrCheckArgs(args)
	if showHelp {
		printPrCheckHelp()
		return 0
	}
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		printPrCheckHelp()
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, runErr := prcheck.Run(ctx, cfg, os.Stdout)
	if runErr != nil {
		if errors.Is(runErr, prcheck.ErrBackendURLMissing) {
			_, _ = fmt.Fprintln(os.Stderr, runErr.Error())
			return 2
		}
		if errors.Is(runErr, prcheck.ErrUnauthorized) {
			_, _ = fmt.Fprintln(os.Stderr, runErr.Error())
			return 2
		}
		if errors.Is(runErr, prcheck.ErrNetworkFailure) {
			_, _ = fmt.Fprintln(os.Stderr, runErr.Error())
			return 2
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", runErr)
		return 2
	}

	if cfg.DryRun {
		return 0
	}

	fmt.Fprintf(os.Stdout, "Uploaded result %s (pass=%d fail=%d); status check posted\n",
		result.ResultID, result.Pass, result.Fail)

	if result.State == "failure" {
		return 1
	}
	return 0
}
```

Updated `printHelp` with new command line:

```go
fmt.Println("  pr-check        Upload test results and post PR status check (Team tier)")
```

New `printPrCheckHelp` function:

```go
func printPrCheckHelp() {
	fmt.Println("Usage: apitest pr-check [options]")
	fmt.Println()
	fmt.Println("Upload test results to the backend and post a PR status check.")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --org <slug>       Organization slug (required)")
	fmt.Println("  --pr <number>      Pull request number (required)")
	fmt.Println("  --repo <owner/repo> Repository in owner/repo format (required)")
	fmt.Println("  --results <file>   Path to test results JSON file (required)")
	fmt.Println("  --dry-run          Print payloads without sending HTTP requests")
	fmt.Println("  --help             Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  APITEST_BACKEND_URL    Backend API base URL (required)")
	fmt.Println("  APITEST_BACKEND_TOKEN  Bearer token for authentication (required)")
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrCheckCmd_Help(t *testing.T) {
	// captureRun(t, "pr-check", "--help")
	// Verify exit 0, stdout contains --org, --pr, --repo, --results, --dry-run,
	// APITEST_BACKEND_URL, APITEST_BACKEND_TOKEN
}

func TestPrCheckCmd_MissingBackendURL(t *testing.T) {
	// No APITEST_BACKEND_URL set
	// Verify exit 2, stderr contains "backend URL not configured"
}

func TestPrCheckCmd_Unauthorized(t *testing.T) {
	// httptest server returning 401
	// Verify exit 2, stderr contains "unauthorized: refresh APITEST_BACKEND_TOKEN"
}

func TestPrCheckCmd_SuccessAllPass(t *testing.T) {
	// httptest server accepting both endpoints
	// Verify exit 0, stdout contains "Uploaded result res_..."
}

func TestPrCheckCmd_FailingTests_Exit1(t *testing.T) {
	// Results file with fail_count > 0
	// Verify exit 1, stdout shows "fail=N"
}

func TestPrCheckCmd_DryRun_NoHTTP(t *testing.T) {
	// --dry-run flag, no server
	// Verify exit 0, stdout contains JSON payloads
}

func TestPrCheckCmd_ConnectionRefused_Exit2(t *testing.T) {
	// Bad URL, verify exit 2 after retries
}

func TestPrCheckCmd_MainHelp_ListsPrCheck(t *testing.T) {
	// captureRun(t, "--help")
	// Verify output contains "pr-check"
}
```

#### Impact on Existing Tests
- `TestHelp` (if it exists and checks for exact help output) may need updating to include `pr-check` in the expected list — but since help tests typically use `strings.Contains`, they should pass as-is. The new `pr-check` line is additive.

---

### Step 6: Create Test Fixtures and Mock Backend

**Rationale:** The task requires `testdata/team/sample-junit.json` and `testdata/team/mock-backend.sh`. These support both manual testing and the observable verification.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/team/sample-junit.json` | create | Sample results file with 3 passing tests |
| `testdata/team/sample-junit-fail.json` | create | Sample results file with 1 failure |
| `testdata/team/mock-backend.sh` | create | Go-based mock HTTP server recording requests |

#### New Code

`testdata/team/sample-junit.json`:
```json
{
  "collection_name": "smoke-tests",
  "run_at": "2026-04-16T10:00:00Z",
  "duration_ms": 1234,
  "pass_count": 3,
  "fail_count": 0,
  "skipped_count": 0,
  "triggered_by": "pr-check",
  "git_sha": "abc1234",
  "items": [
    {"name": "GET /health", "status": "passed", "duration_ms": 120, "message": null},
    {"name": "POST /users", "status": "passed", "duration_ms": 250, "message": null},
    {"name": "GET /users/1", "status": "passed", "duration_ms": 864, "message": null}
  ]
}
```

`testdata/team/mock-backend.sh`:
```bash
#!/usr/bin/env bash
# Tiny Go-based HTTP stub that accepts POST /api/v1/organizations/{org}/results
# and POST /api/v1/organizations/{org}/pr-checks, logging each call.
# Usage: ./mock-backend.sh
# Listens on :18080, writes request log to stdout.
exec go run testdata/team/mock_server.go
```

`testdata/team/mock_server.go`:
```go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		log.Printf("%s %s body=%s", r.Method, r.URL.Path, string(body))

		if r.Method == "POST" && strings.Contains(r.URL.Path, "/results") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			json.NewEncoder(w).Encode(map[string]string{
				"result_id": "res_mock123",
				"status":    "accepted",
			})
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pr-checks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(404)
		fmt.Fprintln(w, "not found")
	})
	log.Println("mock-backend listening on :18080")
	log.Fatal(http.ListenAndServe(":18080", nil))
}
```

#### Tests to Write FIRST (RED phase)
- No unit tests for fixtures; they are validated by the integration tests in Steps 4 and 5.

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 7: Update Smoke Test

**Rationale:** Add a `pr-check --dry-run` invocation to `smoke/run.sh` per definition_of_done.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add pr-check --dry-run smoke test block |

#### Current Code (end of file)

```bash
echo "=== Smoke Test Complete ==="
```

#### New Code (insert before the final line)

```bash
echo "--- PR check dry-run ---"
SMOKE_OUT=$(APITEST_BACKEND_URL=http://localhost:99999 \
  APITEST_BACKEND_TOKEN=fake-token \
  ./apitest pr-check \
    --org acme \
    --pr 42 \
    --repo acme/api \
    --results testdata/team/sample-junit.json \
    --dry-run 2>&1)
SMOKE_RC=$?
if [ "$SMOKE_RC" -eq 0 ]; then
  echo "PASS: pr-check --dry-run exit 0"
else
  echo "FAIL: pr-check --dry-run exit $SMOKE_RC"
  echo "$SMOKE_OUT"
  exit 1
fi
echo "$SMOKE_OUT" | grep -q '"collection_name"' \
  && echo "PASS: dry-run output contains results payload" \
  || { echo "FAIL: missing results payload in dry-run output"; echo "$SMOKE_OUT"; exit 1; }
echo

echo "=== Smoke Test Complete ==="
```

#### Tests to Write FIRST (RED phase)
- N/A — smoke test is a bash script, not a Go test.

#### Impact on Existing Tests
- No existing tests affected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/prcheck/prcheck_test.go` | all | new | write from scratch |
| `internal/prcheck/client_test.go` | all | new | write from scratch |
| `internal/prcheck/run_test.go` | all | new | write from scratch |
| `cmd/apitest/main_test.go` | `TestPrCheck*` | new | add new test functions |
| `cmd/apitest/main_test.go` | existing help tests | none/minor | may need update if help text is checked exactly |

## Risks and Edge Cases

- **Risk:** The `pr-checks` endpoint does not exist on the backend yet. **Mitigation:** The CLI only needs to POST to it — the mock backend stubs the response. When the real endpoint is implemented (possibly in a future task), the CLI will work as-is.

- **Edge case:** Results file with zero items (empty items array but valid structure). **Handling:** Allow it — the pass/fail state is determined by `fail_count`, not by inspecting individual items.

- **Edge case:** Very large results file. **Handling:** Standard `os.ReadFile` is fine for CLI use; the backend enforces the 5 MB limit on its side.

- **Edge case:** Context cancellation during retry backoff. **Handling:** The retry loop checks `ctx.Done()` before sleeping.

- **Edge case:** `--pr 0` or negative PR number. **Handling:** `Validate()` requires `PR > 0`.

- **Risk:** The mock server Go file in `testdata/team/mock_server.go` is in a non-standard location. **Mitigation:** Use `//go:build ignore` build tag or place in a `main` package under testdata (which `go build ./...` ignores by convention since testdata directories are excluded).

- **Edge case:** Backend returns non-JSON error body. **Handling:** The error message includes the raw response body string, which is acceptable for a CLI tool.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/prcheck/... ./cmd/apitest/... -run 'PrCheck' -count=1
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
go build ./cmd/apitest
# Start a local mock of the backend results endpoint
./testdata/team/mock-backend.sh &
MOCK_PID=$!
APITEST_BACKEND_URL=http://localhost:18080 \
APITEST_BACKEND_TOKEN=dev-token \
  ./apitest pr-check \
    --org acme \
    --pr 42 \
    --repo acme/api \
    --results testdata/team/sample-junit.json
# Expected stdout: "Uploaded result res_... (pass=3 fail=0); status check posted"
# Exit 0. Mock log shows POST /api/v1/organizations/acme/results then
# POST /api/v1/organizations/acme/pr-checks with pr=42.
kill $MOCK_PID
go test ./internal/prcheck/... ./cmd/apitest/... -run 'PrCheck' -count=1
# Expected: ok, >=7 tests
```
