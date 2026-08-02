# Implementation Plan: M2-024

## Overview
Extend the retry package with comprehensive trigger condition evaluation: status code matching (exact and ranges), `do_not_retry_on` exclusions (higher precedence), HTTP method restrictions with warnings for non-idempotent methods, network error classification, timeout handling, and the combined condition logic per spec.

## Task Details
- **ID:** M2-024
- **Title:** Advanced retry trigger conditions and method restrictions
- **Phase:** M2: Advanced Retries
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-023 | Backoff strategies (exponential, linear, constant) with jitter | done |

## Architecture Analysis

### Current State
- `IsRetriable(method, result, err)` uses **hardcoded** `DefaultRetriableStatusCodes()` ({429, 502, 503, 504}) and `DefaultIdempotentMethods()` ({GET, HEAD, OPTIONS, TRACE}).
- `Config` (the concrete struct passed to `ExecuteWithRetry`) has no fields for `RetryOn`, `DoNotRetryOn`, or conditions.
- `FullConfig` already has `RetryOn *RetryOnConfig` and `DoNotRetryOn *DoNotRetryOnConfig` with YAML tags and merge support.
- `Resolve()` only transfers scalar fields — it drops `RetryOn` and `DoNotRetryOn`.
- `ExecuteWithRetry` calls `IsRetriable` on line 136 with just method/result/err, ignoring configured conditions.
- The runner calls `ExecuteWithRetry` in 3 places (lines 722, 966, 1126 of `runner.go`).

### Key Design Decision
The `Config` struct needs to carry condition configuration, or `ExecuteWithRetry` needs access to the full merged config. The cleanest approach: **add `RetryOn` and `DoNotRetryOn` fields to `Config`**, update `Resolve()` to populate them, and change `IsRetriable` to accept these conditions.

This avoids changing `ExecuteWithRetry`'s signature (which is called in 3 places in the runner) — instead, the condition data flows through the existing `Config` parameter.

### Warning Mechanism
The spec requires warnings when retrying non-idempotent methods (POST, PATCH). The `Outcome` struct needs a `Warnings []string` field. The runner already surfaces retry count from `Outcome.Attempts`; it can similarly surface warnings.

## Implementation Steps

### Step 1: Add condition types and `ShouldRetry` function
**Rationale:** Core logic first with zero blast radius — new types and a new function, nothing existing changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/condition.go` | create | New file for condition evaluation logic |
| `internal/retry/condition_test.go` | create | Tests for condition evaluation |

#### Tests to Write FIRST (RED phase)

```go
func TestParseStatusRange(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantMin int
        wantMax int
        wantErr bool
    }{
        {"valid range 500-599", "500-599", 500, 599, false},
        {"valid range 400-499", "400-499", 400, 499, false},
        {"single code as range 503-503", "503-503", 503, 503, false},
        {"invalid format no dash", "500", 0, 0, true},
        {"invalid format too many dashes", "500-599-600", 0, 0, true},
        {"invalid min not number", "abc-599", 0, 0, true},
        {"invalid max not number", "500-xyz", 0, 0, true},
        {"min greater than max", "599-500", 0, 0, true},
        {"negative values", "-1-100", 0, 0, true},
        {"empty string", "", 0, 0, true},
    }
    // ...
}

func TestStatusInRange(t *testing.T) {
    tests := []struct {
        name   string
        code   int
        ranges []string
        want   bool
    }{
        {"503 in 500-599", 503, []string{"500-599"}, true},
        {"500 in 500-599 (boundary)", 500, []string{"500-599"}, true},
        {"599 in 500-599 (boundary)", 599, []string{"500-599"}, true},
        {"499 not in 500-599", 499, []string{"500-599"}, false},
        {"600 not in 500-599", 600, []string{"500-599"}, false},
        {"429 in 400-499", 429, []string{"400-499"}, true},
        {"503 in multiple ranges", 503, []string{"400-499", "500-599"}, true},
        {"200 not in any range", 200, []string{"400-499", "500-599"}, false},
        {"empty ranges", 503, nil, false},
        {"invalid range ignored", 503, []string{"invalid"}, false},
    }
    // ...
}

func TestShouldRetry(t *testing.T) {
    tests := []struct {
        name         string
        method       string
        statusCode   int
        isNetworkErr bool
        isTimeout    bool
        retryOn      *RetryOnConfig
        doNotRetryOn *DoNotRetryOnConfig
        wantRetry    bool
        wantWarning  string
    }{
        // Status code exact match
        {"status 503 in retry_on.status_codes", "GET", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, true, ""},
        {"status 400 not in retry_on.status_codes", "GET", 400, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, false, ""},

        // Status range match
        {"status 503 in range 500-599", "GET", 503, false, false,
            &RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}}, nil, true, ""},
        {"status 499 not in range 500-599", "GET", 499, false, false,
            &RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}}, nil, false, ""},

        // Exclusion wins
        {"501 excluded despite 500-599 range", "GET", 501, false, false,
            &RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
            &DoNotRetryOnConfig{StatusCodes: []int{501}}, false, ""},
        {"501 excluded via do_not_retry_on.status_ranges", "GET", 501, false, false,
            &RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
            &DoNotRetryOnConfig{StatusRanges: []string{"501-501"}}, false, ""},

        // Method restrictions
        {"POST not in default methods", "POST", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "HEAD"}}, nil, false, ""},
        {"POST allowed when in methods list", "POST", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "POST"}}, nil, true, "non-idempotent"},
        {"PATCH allowed with warning", "PATCH", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "PATCH"}}, nil, true, "non-idempotent"},
        {"GET no warning", "GET", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}}, nil, true, ""},

        // Method in do_not_retry_on.methods
        {"method excluded by do_not_retry_on", "GET", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}},
            &DoNotRetryOnConfig{Methods: []string{"GET"}}, false, ""},

        // Network errors
        {"network error with network_errors true", "GET", 0, true, false,
            &RetryOnConfig{NetworkErrors: BoolPtr(true), Methods: []string{"GET"}}, nil, true, ""},
        {"network error with network_errors false", "GET", 0, true, false,
            &RetryOnConfig{NetworkErrors: BoolPtr(false), Methods: []string{"GET"}}, nil, false, ""},
        {"network error with network_errors nil (default false)", "GET", 0, true, false,
            &RetryOnConfig{Methods: []string{"GET"}}, nil, false, ""},

        // Timeouts
        {"timeout with timeouts true", "GET", 0, false, true,
            &RetryOnConfig{Timeouts: BoolPtr(true), Methods: []string{"GET"}}, nil, true, ""},
        {"timeout with timeouts false", "GET", 0, false, true,
            &RetryOnConfig{Timeouts: BoolPtr(false), Methods: []string{"GET"}}, nil, false, ""},

        // Combined condition logic
        {"status matches AND method allowed AND NOT excluded", "GET", 503, false, false,
            &RetryOnConfig{StatusRanges: []string{"500-599"}, Methods: []string{"GET"}},
            &DoNotRetryOnConfig{StatusCodes: []int{501}}, true, ""},
        {"PATCH not in default methods no retry despite status match", "PATCH", 503, false, false,
            &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET", "HEAD"}}, nil, false, ""},

        // Nil configs use defaults
        {"nil retryOn uses hardcoded defaults", "GET", 503, false, false,
            nil, nil, true, ""},
        {"nil retryOn POST not retried", "POST", 503, false, false,
            nil, nil, false, ""},
    }
    // ...
}
```

#### New Code

```go
// condition.go

package retry

import (
    "fmt"
    "strconv"
    "strings"
)

// ShouldRetryInput captures the state needed to evaluate retry conditions.
type ShouldRetryInput struct {
    Method       string
    StatusCode   int  // 0 when no HTTP response (network error / timeout)
    IsNetworkErr bool
    IsTimeout    bool
}

// ShouldRetryResult holds the retry decision and optional warning.
type ShouldRetryResult struct {
    Retry   bool
    Warning string // non-empty when retrying a non-idempotent method
}

// nonIdempotentMethods are HTTP methods that are not idempotent.
var nonIdempotentMethods = map[string]bool{
    "POST": true, "PATCH": true,
}

// ShouldRetry evaluates the combined condition logic per spec:
//
//   (status_code IN retry_on.status_codes OR status_code IN retry_on.status_ranges
//    OR network_error AND retry_on.network_errors
//    OR timeout AND retry_on.timeouts)
//   AND HTTP_method IN retry_on.methods
//   AND NOT (status_code IN do_not_retry_on.status_codes OR status_code IN do_not_retry_on.status_ranges
//            OR HTTP_method IN do_not_retry_on.methods)
//
// When retryOn is nil, uses DefaultRetriableStatusCodes and DefaultIdempotentMethods.
func ShouldRetry(input ShouldRetryInput, retryOn *RetryOnConfig, doNotRetryOn *DoNotRetryOnConfig) ShouldRetryResult {
    // Fall back to defaults when no retry_on config
    if retryOn == nil {
        retry := DefaultIdempotentMethods()[input.Method] &&
            (DefaultRetriableStatusCodes()[input.StatusCode] || input.IsNetworkErr || input.IsTimeout)
        return ShouldRetryResult{Retry: retry}
    }

    // 1. Check trigger conditions (OR logic)
    triggered := false
    if input.StatusCode > 0 {
        triggered = statusInCodes(input.StatusCode, retryOn.StatusCodes) ||
            StatusInRanges(input.StatusCode, retryOn.StatusRanges)
    }
    if !triggered && input.IsNetworkErr && retryOn.NetworkErrors != nil && *retryOn.NetworkErrors {
        triggered = true
    }
    if !triggered && input.IsTimeout && retryOn.Timeouts != nil && *retryOn.Timeouts {
        triggered = true
    }
    if !triggered {
        return ShouldRetryResult{Retry: false}
    }

    // 2. Check method restriction (AND)
    if !methodAllowed(input.Method, retryOn.Methods) {
        return ShouldRetryResult{Retry: false}
    }

    // 3. Check exclusions (AND NOT)
    if doNotRetryOn != nil {
        if input.StatusCode > 0 &&
            (statusInCodes(input.StatusCode, doNotRetryOn.StatusCodes) ||
                StatusInRanges(input.StatusCode, doNotRetryOn.StatusRanges)) {
            return ShouldRetryResult{Retry: false}
        }
        if methodExcluded(input.Method, doNotRetryOn.Methods) {
            return ShouldRetryResult{Retry: false}
        }
    }

    // 4. Warning for non-idempotent methods
    var warning string
    if nonIdempotentMethods[input.Method] {
        warning = fmt.Sprintf("retrying non-idempotent method %s", input.Method)
    }

    return ShouldRetryResult{Retry: true, Warning: warning}
}

// ParseStatusRange parses "min-max" into two ints.
func ParseStatusRange(s string) (int, int, error) {
    parts := strings.SplitN(s, "-", 2)
    if len(parts) != 2 {
        return 0, 0, fmt.Errorf("invalid status range %q: expected min-max", s)
    }
    // Guard against "500-599-600" by checking for additional dashes
    if strings.Count(s, "-") > 1 {
        return 0, 0, fmt.Errorf("invalid status range %q: too many dashes", s)
    }
    min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
    if err != nil {
        return 0, 0, fmt.Errorf("invalid status range %q: min: %w", s, err)
    }
    max, err := strconv.Atoi(strings.TrimSpace(parts[1]))
    if err != nil {
        return 0, 0, fmt.Errorf("invalid status range %q: max: %w", s, err)
    }
    if min < 0 || max < 0 {
        return 0, 0, fmt.Errorf("invalid status range %q: negative values", s)
    }
    if min > max {
        return 0, 0, fmt.Errorf("invalid status range %q: min > max", s)
    }
    return min, max, nil
}

// StatusInRanges checks whether code falls within any of the given ranges.
func StatusInRanges(code int, ranges []string) bool {
    for _, r := range ranges {
        min, max, err := ParseStatusRange(r)
        if err != nil {
            continue // skip invalid ranges
        }
        if code >= min && code <= max {
            return true
        }
    }
    return false
}

func statusInCodes(code int, codes []int) bool {
    for _, c := range codes {
        if c == code {
            return true
        }
    }
    return false
}

func methodAllowed(method string, methods []string) bool {
    if len(methods) == 0 {
        // No methods specified = default idempotent methods
        return DefaultIdempotentMethods()[method]
    }
    for _, m := range methods {
        if strings.EqualFold(m, method) {
            return true
        }
    }
    return false
}

func methodExcluded(method string, methods []string) bool {
    for _, m := range methods {
        if strings.EqualFold(m, method) {
            return true
        }
    }
    return false
}
```

#### Impact on Existing Tests
- No existing tests affected — this is a new file.

### Step 2: Extend `Config` and `Resolve()` to carry condition fields
**Rationale:** Small, backward-compatible change. `Config` gains optional fields; existing callers that don't set them get the same behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/config.go` | modify | Add `RetryOn` and `DoNotRetryOn` fields to `Config`; update `Resolve()` |
| `internal/retry/merge_test.go` | modify | Add test for `Resolve()` with condition fields |

#### Current Code
```go
// Config struct (config.go lines 14-22)
type Config struct {
    Enabled         bool    `yaml:"enabled"`
    MaxAttempts     int     `yaml:"max_attempts"`
    InitialDelayMs  int     `yaml:"initial_delay_ms"`
    BackoffStrategy string  `yaml:"backoff_strategy"`
    MaxDelayMs      int     `yaml:"max_delay_ms"`
    Jitter          bool    `yaml:"jitter"`
    JitterFactor    float64 `yaml:"jitter_factor"`
}

// Resolve method (config.go lines 81-105) — does not transfer RetryOn/DoNotRetryOn
```

#### New Code
```go
type Config struct {
    Enabled         bool    `yaml:"enabled"`
    MaxAttempts     int     `yaml:"max_attempts"`
    InitialDelayMs  int     `yaml:"initial_delay_ms"`
    BackoffStrategy string  `yaml:"backoff_strategy"`
    MaxDelayMs      int     `yaml:"max_delay_ms"`
    Jitter          bool    `yaml:"jitter"`
    JitterFactor    float64 `yaml:"jitter_factor"`
    RetryOn         *RetryOnConfig      // nil = use defaults
    DoNotRetryOn    *DoNotRetryOnConfig // nil = no exclusions
}

// Updated Resolve():
func (fc FullConfig) Resolve() Config {
    // ... existing scalar transfers ...
    if fc.RetryOn != nil {
        cp := *fc.RetryOn
        c.RetryOn = &cp
    }
    if fc.DoNotRetryOn != nil {
        cp := *fc.DoNotRetryOn
        c.DoNotRetryOn = &cp
    }
    return c
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestFullConfig_Resolve_conditions(t *testing.T) {
    tests := []struct {
        name string
        fc   FullConfig
        checkRetryOn      bool
        checkDoNotRetryOn bool
    }{
        {"retryOn transferred", FullConfig{
            RetryOn: &RetryOnConfig{StatusCodes: []int{503}, Methods: []string{"GET"}},
        }, true, false},
        {"doNotRetryOn transferred", FullConfig{
            DoNotRetryOn: &DoNotRetryOnConfig{StatusCodes: []int{501}},
        }, false, true},
        {"nil stays nil", FullConfig{}, false, false},
    }
    // ...
}
```

#### Impact on Existing Tests
- `TestFullConfig_Resolve` — existing tests use `Config` value comparison which works because `Config` zero value for new pointer fields is nil. **No breakage.**
- However, if any test uses `Config{...}` literal comparison, the addition of new fields may cause compilation issues only if named-field initialization was used incompletely. Current tests use `Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 2000}` which is named fields — safe, no breakage.

### Step 3: Add `Warnings` field to `Outcome` and update `ExecuteWithRetry`
**Rationale:** Wire the new `ShouldRetry` into the retry loop, replacing the hardcoded `IsRetriable`. This is the core integration step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/retry.go` | modify | Add `Warnings` to `Outcome`, replace `IsRetriable` call with `ShouldRetry` in `ExecuteWithRetry` |
| `internal/retry/retry_test.go` | modify | Update tests to cover new condition-based retries |

#### Current Code
```go
// Outcome struct (retry.go lines 18-22)
type Outcome struct {
    Result   *httpexec.Result
    Err      error
    Attempts int
}

// retry loop (retry.go line 136)
for attempts < maxAttempts && IsRetriable(method, result, err) {
```

#### New Code
```go
type Outcome struct {
    Result   *httpexec.Result
    Err      error
    Attempts int
    Warnings []string // warnings (e.g., non-idempotent method retry)
}

// In ExecuteWithRetry, replace the IsRetriable call:
for attempts < maxAttempts {
    input := classifyForRetry(method, result, err)
    decision := ShouldRetry(input, cfg.RetryOn, cfg.DoNotRetryOn)
    if !decision.Retry {
        break
    }
    if decision.Warning != "" {
        warnings = append(warnings, decision.Warning)
    }
    // ... delay/sleep/exec ...
}
```

Where `classifyForRetry` is a helper:
```go
func classifyForRetry(method string, result *httpexec.Result, err error) ShouldRetryInput {
    input := ShouldRetryInput{Method: method}
    if result != nil {
        input.StatusCode = result.StatusCode
    }
    if err != nil {
        var netErr *apierrors.NetworkError
        if errors.As(err, &netErr) {
            if netErr.Kind == apierrors.NetworkTimeout {
                input.IsTimeout = true
            } else {
                input.IsNetworkErr = true
            }
        }
    }
    return input
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWithRetry_statusRangeRetry(t *testing.T) {
    // Configure retry_on.status_ranges: ["500-599"], verify 503 retries
}

func TestExecuteWithRetry_doNotRetryOnExclusion(t *testing.T) {
    // Configure retry_on.status_ranges: ["500-599"], do_not_retry_on.status_codes: [501]
    // Verify 501 does NOT retry
}

func TestExecuteWithRetry_methodRestriction(t *testing.T) {
    // POST with retry_on.methods: [GET, POST] retries with warning
}

func TestExecuteWithRetry_nonIdempotentWarning(t *testing.T) {
    // POST retry produces warning in Outcome.Warnings
}

func TestExecuteWithRetry_networkErrorCondition(t *testing.T) {
    // retry_on.network_errors: true, connection refused, verify retry
}

func TestExecuteWithRetry_timeoutCondition(t *testing.T) {
    // retry_on.timeouts: true, timeout error, verify retry with full timeout per attempt
}

func TestExecuteWithRetry_defaultBehaviorUnchanged(t *testing.T) {
    // nil RetryOn/DoNotRetryOn = same behavior as before (hardcoded defaults)
}

func TestClassifyForRetry(t *testing.T) {
    tests := []struct {
        name       string
        method     string
        result     *httpexec.Result
        err        error
        wantInput  ShouldRetryInput
    }{
        {"GET 503", "GET", &httpexec.Result{StatusCode: 503}, nil,
            ShouldRetryInput{Method: "GET", StatusCode: 503}},
        {"network error", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS},
            ShouldRetryInput{Method: "GET", IsNetworkErr: true}},
        {"timeout error", "GET", nil, &apierrors.NetworkError{Kind: apierrors.NetworkTimeout},
            ShouldRetryInput{Method: "GET", IsTimeout: true}},
        {"nil result nil error", "GET", nil, nil,
            ShouldRetryInput{Method: "GET"}},
    }
}
```

#### Impact on Existing Tests
- **`TestIsRetriable`** — `IsRetriable` still exists (kept for backward compat and as a simplified wrapper). No breakage.
- **`TestExecuteWithRetry`** — All existing tests pass nil `RetryOn`/`DoNotRetryOn` in `Config`, which triggers default behavior (same as current hardcoded logic). **No breakage** as long as `ShouldRetry` with nil `retryOn` matches `IsRetriable` behavior.
- **`TestExecuteWithRetry_linearBackoff`**, **`TestExecuteWithRetry_constantBackoff`**, etc. — Same: Config has no RetryOn set, so defaults apply. **No breakage.**
- **Runner tests** — They construct `retry.Config` via `Resolve()`. Since `FullConfig` in those tests don't set `RetryOn`, the resolved `Config.RetryOn` is nil → default behavior. **No breakage.**

### Step 4: Surface warnings in the runner (optional, minimal)
**Rationale:** The `Outcome.Warnings` field needs to be accessible to the output layer. Add a `Warnings` field to `RequestResult`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Pass `Outcome.Warnings` through to `RequestResult` |

#### Current Code
```go
// runner.go line 730
retryCount := outcome.Attempts - 1
```

#### New Code
```go
retryCount := outcome.Attempts - 1
retryWarnings := outcome.Warnings
// ... later when building RequestResult:
// Warnings: retryWarnings,
```

And add to `RequestResult`:
```go
type RequestResult struct {
    // ... existing fields ...
    Warnings []string // retry warnings (e.g., non-idempotent method)
}
```

#### Tests to Write FIRST (RED phase)
No new runner-level tests needed for just wiring warnings through. The condition evaluation is fully tested in the retry package. Runner integration tests for retry already exist and will continue to pass.

#### Impact on Existing Tests
- `RequestResult` gains a new field `Warnings []string`. Existing tests using named field initialization are unaffected (zero value is nil). **No breakage.**

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/retry/retry_test.go` | `TestIsRetriable` | none | no change needed |
| `internal/retry/retry_test.go` | `TestExecuteWithRetry` | none | default behavior preserved |
| `internal/retry/retry_test.go` | `TestExecuteWithRetry_*Backoff` | none | default behavior preserved |
| `internal/retry/merge_test.go` | `TestFullConfig_Resolve` | none | named fields, nil defaults |
| `internal/runner/runner_test.go` | retry-related tests | none | `Config.RetryOn` nil = defaults |

## Risks and Edge Cases

- **Risk:** Changing `ExecuteWithRetry`'s internal loop could break the existing retry behavior for cases without explicit `retry_on` config.
  → **Mitigation:** When `Config.RetryOn` is nil, `ShouldRetry` falls back to `DefaultRetriableStatusCodes()` and `DefaultIdempotentMethods()`, exactly matching current `IsRetriable` behavior. All existing tests serve as regression tests.

- **Edge case:** Status range "500-599" with negative numbers like "-1-100".
  → **Handling:** `ParseStatusRange` rejects negative values with an error.

- **Edge case:** Invalid range format (e.g., "abc-def", "500") silently ignored.
  → **Handling:** `StatusInRanges` skips invalid ranges (logged as parse error but non-fatal).

- **Edge case:** Empty `retry_on.methods` list vs nil.
  → **Handling:** When `retryOn` is non-nil but `methods` is empty/nil, fall back to `DefaultIdempotentMethods()`. This matches the spec — methods default to idempotent-only.

- **Risk:** `Config` struct gains pointer fields, breaking value equality comparison in tests.
  → **Mitigation:** Current `TestFullConfig_Resolve` tests use `Config{...}` value comparison. Since new pointer fields default to nil, `Config{Enabled: true, ...} == Config{Enabled: true, ..., RetryOn: nil}` is true. However, pointer fields prevent `==` comparison entirely in Go. **Must update `TestFullConfig_Resolve` to use field-by-field comparison instead of `got != tt.want`.**

- **Edge case:** Both `retry_on.status_codes` and `retry_on.status_ranges` set — OR logic means matching either triggers retry.
  → **Handling:** Explicit OR in condition evaluation.

- **Risk:** The truth table row "Server error PUT | YES" implies PUT is in the methods list.
  → **Handling:** PUT is NOT in default methods. The truth table example assumes explicit `retry_on.methods` including PUT. Our defaults match spec: only GET/HEAD/OPTIONS/TRACE.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Run condition evaluation tests
go test ./internal/retry/... -v -run "TestShouldRetry|TestParseStatusRange|TestStatusInRange"

# Run ExecuteWithRetry integration tests with conditions
go test ./internal/retry/... -v -run "TestExecuteWithRetry_statusRange|TestExecuteWithRetry_doNotRetry|TestExecuteWithRetry_method|TestExecuteWithRetry_network|TestExecuteWithRetry_timeout"

# Verify test coverage
go test -coverprofile=coverage.out ./internal/retry/...
go tool cover -func=coverage.out | grep retry
```
