# Implementation Plan: M2-013

## Overview

Add basic retry logic with configurable max attempts. Requests that fail with transient errors (503, 502, 429, 504, network errors) are automatically retried up to a configurable `max_attempts`, with exponential backoff and Retry-After header support. Retry is gated to Solo tier.

## Task Details
- **ID:** M2-013
- **Title:** Basic retry logic with configurable max attempts
- **Phase:** M2: Retries
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | Parse and execute a single GET request | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Create `internal/retry/` package with core retry logic

**Rationale:** Isolated package with no integration dependencies — can be fully unit-tested before touching any existing code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/config.go` | create | Config struct, defaults, retriable conditions |
| `internal/retry/retry.go` | create | ExecuteWithRetry loop, backoff, Retry-After parsing |
| `internal/retry/retry_test.go` | create | Comprehensive unit tests |

#### New Code

```go
// config.go
package retry

import "net/http"

// Config represents retry configuration parsed from YAML.
type Config struct {
	Enabled        bool `yaml:"enabled"`
	MaxAttempts    int  `yaml:"max_attempts"`
	InitialDelayMs int  `yaml:"initial_delay_ms"`
}

// DefaultConfig returns the built-in defaults (disabled, 3 attempts, 1000ms initial delay).
func DefaultConfig() Config {
	return Config{
		Enabled:        false,
		MaxAttempts:    3,
		InitialDelayMs: 1000,
	}
}

// DefaultRetriableStatusCodes returns status codes that trigger retries.
func DefaultRetriableStatusCodes() map[int]bool {
	return map[int]bool{429: true, 502: true, 503: true, 504: true}
}

// DefaultIdempotentMethods returns HTTP methods safe to retry.
func DefaultIdempotentMethods() map[string]bool {
	return map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}
}
```

```go
// retry.go
package retry

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
)

// Outcome holds the result of a retried execution.
type Outcome struct {
	Result   *httpexec.Result
	Err      error
	Attempts int // total attempts (1 = no retries)
}

// SleepFunc is an injectable sleep for testing.
type SleepFunc func(ctx context.Context, d time.Duration) error

// DefaultSleep sleeps for d or until ctx is cancelled.
func DefaultSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsRetriable determines whether a failed request should be retried.
func IsRetriable(method string, result *httpexec.Result, err error) bool {
	if !DefaultIdempotentMethods()[method] {
		return false
	}
	if err != nil {
		var netErr *apierrors.NetworkError
		if errors.As(err, &netErr) {
			return true
		}
		return false
	}
	if result != nil {
		return DefaultRetriableStatusCodes()[result.StatusCode]
	}
	return false
}

// ParseRetryAfter extracts delay from Retry-After header.
// Returns 0 if absent, empty, or unparseable. Caps at 30s.
func ParseRetryAfter(headers http.Header) time.Duration

// BackoffDelay computes exponential backoff: initialDelayMs * 2^attempt, capped at 30s.
func BackoffDelay(attempt int, initialDelayMs int) time.Duration

// ExecuteWithRetry wraps exec with retry logic.
// If cfg.Enabled is false, exec is called exactly once.
func ExecuteWithRetry(
	ctx context.Context,
	cfg Config,
	method string,
	exec func(ctx context.Context) (*httpexec.Result, error),
	sleep SleepFunc,
) *Outcome
```

#### Tests to Write FIRST (RED phase)

```go
func TestIsRetriable(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		result  *httpexec.Result
		err     error
		want    bool
	}{
		{"GET 503 is retriable", "GET", &httpexec.Result{StatusCode: 503}, nil, true},
		{"GET 502 is retriable", "GET", &httpexec.Result{StatusCode: 502}, nil, true},
		{"GET 429 is retriable", "GET", &httpexec.Result{StatusCode: 429}, nil, true},
		{"GET 504 is retriable", "GET", &httpexec.Result{StatusCode: 504}, nil, true},
		{"GET 400 not retriable", "GET", &httpexec.Result{StatusCode: 400}, nil, false},
		{"GET 401 not retriable", "GET", &httpexec.Result{StatusCode: 401}, nil, false},
		{"GET 404 not retriable", "GET", &httpexec.Result{StatusCode: 404}, nil, false},
		{"GET 200 not retriable", "GET", &httpexec.Result{StatusCode: 200}, nil, false},
		{"POST 503 not retriable", "POST", &httpexec.Result{StatusCode: 503}, nil, false},
		{"PATCH 502 not retriable", "PATCH", &httpexec.Result{StatusCode: 502}, nil, false},
		{"GET network error retriable", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS}, true},
		{"GET connection refused retriable", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkConnectionRefused}, true},
		{"POST network error not retriable", "POST", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS}, false},
	}
	// ...
}

func TestBackoffDelay(t *testing.T) {
	tests := []struct {
		name           string
		attempt        int
		initialDelayMs int
		want           time.Duration
	}{
		{"attempt 0 = initial delay", 0, 1000, 1 * time.Second},
		{"attempt 1 = 2x initial", 1, 1000, 2 * time.Second},
		{"attempt 2 = 4x initial", 2, 1000, 4 * time.Second},
		{"capped at 30s", 10, 1000, 30 * time.Second},
	}
	// ...
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"integer seconds", "5", 5 * time.Second},
		{"zero", "0", 0},
		{"missing header", "", 0},
		{"invalid value", "abc", 0},
		{"capped at 30s", "120", 30 * time.Second},
	}
	// ...
}

func TestExecuteWithRetry(t *testing.T) {
	tests := []struct {
		name string
		// ...
	}{
		{"disabled calls exec once"},
		{"succeeds first attempt - attempts=1"},
		{"retries on 503 then succeeds - attempts=2"},
		{"all attempts fail - returns last error after max_attempts"},
		{"retries on network error"},
		{"Retry-After header respected"},
		{"context cancelled stops retrying"},
		{"POST with 503 no retry"},
		{"GET 400 no retry"},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

---

### Step 2: Add RetryConfig to parser types and YAML parsing

**Rationale:** Additive change only — zero-value Config means disabled, so all existing collections parse identically.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Retry` field to `Collection` and `RequestItem` |
| `internal/parser/parser_test.go` | modify | Add tests for retry config parsing |
| `internal/parser/testdata/with_retry.yaml` | create | Test collection with collection-level retry |
| `internal/parser/testdata/with_request_retry.yaml` | create | Test collection with per-request retry |

#### Current Code
```go
// Collection represents a parsed collection file.
type Collection struct {
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Variables     SensitiveVars `yaml:"variables,omitempty"`
	Setup         []RequestItem `yaml:"setup,omitempty"`
	Requests      []RequestItem `yaml:"requests"`
	Teardown      []RequestItem `yaml:"teardown,omitempty"`
	Options       Options       `yaml:"options,omitempty"`
	ExternalFiles []string      `yaml:"-"`
}

// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Path       string            `yaml:"path,omitempty"`
	Auth       string            `yaml:"auth,omitempty"`
	Required   *bool             `yaml:"required,omitempty"`
	Request    Request           `yaml:"request"`
	Variables  SensitiveVars     `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
import "github.com/weiqigod/curlew/internal/retry"

type Collection struct {
	Name          string        `yaml:"name"`
	Description   string        `yaml:"description,omitempty"`
	Variables     SensitiveVars `yaml:"variables,omitempty"`
	Retry         retry.Config  `yaml:"retry,omitempty"`        // collection-level retry
	Setup         []RequestItem `yaml:"setup,omitempty"`
	Requests      []RequestItem `yaml:"requests"`
	Teardown      []RequestItem `yaml:"teardown,omitempty"`
	Options       Options       `yaml:"options,omitempty"`
	ExternalFiles []string      `yaml:"-"`
}

type RequestItem struct {
	Name       string            `yaml:"name"`
	Path       string            `yaml:"path,omitempty"`
	Auth       string            `yaml:"auth,omitempty"`
	Required   *bool             `yaml:"required,omitempty"`
	Retry      *retry.Config     `yaml:"retry,omitempty"`       // per-request override (nil = inherit)
	Request    Request           `yaml:"request"`
	Variables  SensitiveVars     `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_withRetryConfig(t *testing.T) {
	// Parse testdata/with_retry.yaml
	// Verify col.Retry.Enabled == true
	// Verify col.Retry.MaxAttempts == 3
}

func TestParseFile_withRequestRetry(t *testing.T) {
	// Parse testdata/with_request_retry.yaml
	// Verify per-request retry config overrides collection
	// Verify requests without retry: have nil Retry
}

func TestParseFile_retryDefaultsWhenOmitted(t *testing.T) {
	// Parse existing testdata collection without retry:
	// Verify col.Retry == retry.Config{} (zero value, disabled)
}
```

#### Impact on Existing Tests
- Existing parser tests are unaffected. The `Retry` field zero-value (all false/0) is equivalent to "no retry config present" since YAML `omitempty` skips zero values.

---

### Step 3: Register retry as Solo-tier feature gate

**Rationale:** One-line additive change in registry, enables gate check in Step 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add retry feature definition |
| `internal/auth/gate_test.go` | modify | Add test for retry gate |

#### Current Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(FeatureDefinition{
		Name:         "vault_provider_profiles",
		// ...
	})
	r.Register(FeatureDefinition{
		Name:         "from_command",
		// ...
	})
	r.Register(FeatureDefinition{
		Name:         "dynamic_auth_profiles",
		// ...
	})
	return r
}
```

#### New Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	// ... existing registrations ...
	r.Register(FeatureDefinition{
		Name:         "retry",
		RequiredTier: TierSolo,
		Description:  "Retry logic requires Solo tier ($9/month)",
		Workaround:   "Run requests manually or use shell scripting for retry logic",
	})
	return r
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature_retry(t *testing.T) {
	tests := []struct {
		name    string
		tier    Tier
		wantErr bool
	}{
		{"free tier blocked", TierFree, true},
		{"solo tier allowed", TierSolo, false},
		{"professional tier allowed", TierProfessional, false},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected (additive registration)

---

### Step 4: Integrate retry into runner's execution loop

**Rationale:** Core integration point. Steps 1-3 must be complete before this.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Wrap exec call with retry, add RetryCount to RequestResult, pass retry config through executePhase |
| `internal/runner/runner_test.go` | modify | Add retry integration tests |

#### Current Code (runner.go:520)
```go
result, execErr := exec(ctx, &req)
*counter++
```

#### New Code
```go
// Determine effective retry config: request-level overrides collection-level.
retryCfg := effectiveRetryConfig(collectionRetry, item.Retry)

// Feature gate check (only if retry is enabled).
if retryCfg.Enabled {
	if gateErr := auth.CheckFeature(reg, "retry", tier); gateErr != nil {
		return results, requiredFailed, gateErr
	}
}

outcome := retry.ExecuteWithRetry(ctx, retryCfg, req.Method, func(rCtx context.Context) (*httpexec.Result, error) {
	r, e := exec(rCtx, &req)
	*counter++
	return r, e
}, retry.DefaultSleep)

result := outcome.Result
execErr := outcome.Err
retryCount := outcome.Attempts - 1
```

**Additional changes to runner.go:**

1. Add `RetryCount int` field to `RequestResult` struct (line 107)
2. Add `collectionRetry retry.Config` parameter to `executePhase` signature (line 443)
3. Pass `reg *auth.Registry` and `tier auth.Tier` to `executePhase` (already available in the caller)
4. Set `rr.RetryCount = retryCount` when building `RequestResult` (lines 553, 577)
5. Add helper function:
```go
func effectiveRetryConfig(collection retry.Config, request *retry.Config) retry.Config {
	if request != nil {
		return *request
	}
	return collection
}
```

**Interaction with auth 401 refresh (lines 523-550):** The retry loop wraps the individual `exec` call. 401 is NOT in retriable status codes, so the retry loop returns immediately on 401. The existing auth refresh logic then operates on the final outcome — no conflict.

#### Tests to Write FIRST (RED phase)

```go
func TestExecutePhase_retryOn503(t *testing.T) {
	// Mock exec returns 503 twice, then 200
	// Verify result shows passed with RetryCount=2
}

func TestExecutePhase_retryAllFail(t *testing.T) {
	// Mock exec returns 503 three times
	// Verify last failure reported, RetryCount=2
}

func TestExecutePhase_retryDisabled(t *testing.T) {
	// No retry config, exec returns 503
	// Verify no retry (Attempts=1)
}

func TestExecutePhase_retryNetworkError(t *testing.T) {
	// Mock exec returns NetworkError then success
	// Verify retried and succeeded
}

func TestExecutePhase_retryPOST503(t *testing.T) {
	// POST request with 503, retry enabled
	// Verify NOT retried (non-idempotent)
}

func TestExecutePhase_retryFeatureGate(t *testing.T) {
	// Free tier with retry enabled
	// Verify GateError returned (exit code 6)
}

func TestRun_retryCountInResult(t *testing.T) {
	// Full Run() call with retry, verify RetryCount propagated
}
```

#### Impact on Existing Tests
- `executePhase` signature changes — all existing callers in `runner_test.go` need the new `collectionRetry retry.Config` parameter (zero-value = disabled, no behavior change)
- All existing callers also need `reg *auth.Registry` and `tier auth.Tier` parameters if not already passed (check actual caller signatures)
- `RequestResult` gains `RetryCount int` — existing equality checks may need updating if using struct literals

---

### Step 5: Update output formatters for retry count display

**Rationale:** User-facing change, depends on RetryCount being available in RequestResult (Step 4).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add retry count to `Result()` output |
| `internal/output/terminal_test.go` | modify | Test retry count display |
| `internal/output/json.go` | modify | Add `RetryCount` to `JSONRequest` |
| `internal/output/tap.go` | modify | Add `RetryCount` to `TAPResult` |
| `cmd/curlew/main.go` | modify | Wire RetryCount from RequestResult to output structs |

#### Current Code (terminal.go:34)
```go
func (p *Printer) Result(name string, result *httpexec.Result, passed bool) {
	// ...
	_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms\n",
		colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds())
}
```

#### New Code (terminal.go)
```go
func (p *Printer) Result(name string, result *httpexec.Result, passed bool, retryCount int) {
	// ...
	if retryCount > 0 {
		_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms (retry: %d)\n",
			colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds(), retryCount)
	} else {
		_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms\n",
			colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds())
	}
}
```

#### JSON Output Change
```go
type JSONRequest struct {
	// ... existing fields ...
	RetryCount int `json:"retry_count,omitempty"`
}
```

#### TAP Output Change
```go
type TAPResult struct {
	// ... existing fields ...
	RetryCount int // included in YAML diagnostics if > 0
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestResult_withRetryCount(t *testing.T) {
	// Verify "(retry: 2)" appended when retryCount=2
}

func TestResult_noRetryCount(t *testing.T) {
	// Verify no retry suffix when retryCount=0
}

func TestJSONRequest_retryCountOmitted(t *testing.T) {
	// Verify retry_count not in JSON when 0
}

func TestJSONRequest_retryCountPresent(t *testing.T) {
	// Verify retry_count in JSON when > 0
}
```

#### Impact on Existing Tests
- `Printer.Result()` gains a `retryCount int` parameter — all existing callers (terminal_test.go, main.go) must add `, 0` as the last argument
- `buildJSONOutput` and `buildTAPOutput` in main.go need to read `r.RetryCount` and set it on the output structs

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | All `executePhase` callers | signature change | Add `collectionRetry`, `reg`, `tier` params |
| `internal/runner/runner_test.go` | `RequestResult` comparisons | new field | Add `RetryCount: 0` to struct literals |
| `internal/output/terminal_test.go` | `TestResult*` | signature change | Add `, 0` to `Result()` calls |
| `internal/parser/parser_test.go` | `TestParseFile*` | none | Zero-value Retry on existing tests |
| `internal/auth/gate_test.go` | existing gate tests | none | Additive registration |
| `cmd/curlew/main.go` | `out.Result()` calls | signature change | Add `, r.RetryCount` |

## Risks and Edge Cases

- **Risk:** Retry-After header in HTTP-date format vs integer seconds
  **Mitigation:** Parse both formats; HTTP-date in the past = 0; cap at 30s safety limit

- **Risk:** Context cancellation during retry wait (user Ctrl+C)
  **Mitigation:** `DefaultSleep` uses `select` on `ctx.Done()` and `time.After`; returns `ctx.Err()` immediately

- **Risk:** Guard rail counter interaction — retries consume request quota
  **Mitigation:** Each retry attempt increments `*counter` (correct: retries ARE real HTTP requests). If limit hit mid-retry, context cancellation stops further attempts.

- **Risk:** POST with retry enabled could cause duplicate side effects
  **Mitigation:** `IsRetriable` rejects non-idempotent methods (POST, PATCH, PUT, DELETE) by default

- **Risk:** Auth 401 refresh vs retry interaction
  **Mitigation:** 401 is NOT in retriable status codes. Retry returns immediately on 401, and existing auth refresh logic runs after.

- **Risk:** Variable extraction after retry — which response?
  **Mitigation:** The final outcome (successful or last failed). Retry loop returns the last `Result`, which is passed to extraction.

- **Risk:** Assertion evaluation timing
  **Mitigation:** Assertions run only on the final outcome after retries (not intermediate attempts). This matches spec: "Retries happen within request execution (before extracting variables)."

- **Edge case:** Exponential backoff overflow with large attempt numbers
  **Mitigation:** `BackoffDelay` caps at 30s max. Use safe math for `2^attempt`.

- **Edge case:** Retry-After with very large value (e.g., 3600)
  **Mitigation:** Cap ParseRetryAfter at 30s. Log/warn if original value exceeded cap.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a test collection targeting a flaky endpoint:
cat > /tmp/retry_test.yaml << 'EOF'
name: Retry Test
retry:
  enabled: true
  max_attempts: 3
requests:
  - name: Flaky endpoint
    request:
      method: GET
      url: https://httpbin.org/status/503
    assertions:
      status: 503
EOF

curlew run /tmp/retry_test.yaml
# Confirm retries occur (visible in output as retry count)

go test ./internal/retry/... -v
```
