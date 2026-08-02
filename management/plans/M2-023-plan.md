# Implementation Plan: M2-023

## Overview
Extend the retry package with three backoff strategy implementations (exponential, linear, constant), jitter calculation with configurable factor, max_delay_ms capping, and feature gating so that custom backoff strategies (linear, constant) require Professional tier.

## Task Details
- **ID:** M2-023
- **Title:** Backoff strategies (exponential, linear, constant) with jitter
- **Phase:** M2: Advanced Retries
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-013 | Basic retry logic | done |
| M1-028 | Feature gate framework | done |

## Implementation Steps

### Step 1: Add backoff strategy types and delay calculation function
**Rationale:** Pure functions with no side effects — smallest blast radius, no existing code changes, all new code.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/backoff.go` | create | Strategy type, CalculateDelay function, ApplyJitter function |
| `internal/retry/backoff_test.go` | create | Table-driven tests for all strategies, jitter, and capping |

#### New Code
```go
package retry

import (
	"math"
	"math/rand"
	"time"
)

// Strategy represents a backoff strategy type.
type Strategy string

const (
	StrategyExponential Strategy = "exponential"
	StrategyLinear      Strategy = "linear"
	StrategyConstant    Strategy = "constant"
)

// ValidStrategy reports whether s is a recognized backoff strategy.
func ValidStrategy(s string) bool {
	switch Strategy(s) {
	case StrategyExponential, StrategyLinear, StrategyConstant:
		return true
	}
	return false
}

// CalculateDelay computes the delay for the given attempt using the specified strategy.
// attempt is 0-indexed (0 = first retry delay).
// The result is capped at maxDelayMs. If maxDelayMs <= 0, defaults to 30000.
func CalculateDelay(strategy Strategy, attempt, initialDelayMs, maxDelayMs int) time.Duration {
	if maxDelayMs <= 0 {
		maxDelayMs = 30000
	}
	var ms float64
	switch strategy {
	case StrategyLinear:
		// delay = initial_delay_ms * (attempt + 1)
		ms = float64(initialDelayMs) * float64(attempt+1)
	case StrategyConstant:
		ms = float64(initialDelayMs)
	default: // exponential
		ms = float64(initialDelayMs) * math.Pow(2, float64(attempt))
	}
	maxMs := float64(maxDelayMs)
	if ms > maxMs {
		ms = maxMs
	}
	return time.Duration(ms) * time.Millisecond
}

// JitterFunc is a function that returns a random float64 in [-1, 1).
// Injectable for deterministic testing.
type JitterFunc func() float64

// DefaultJitter returns a random float64 in [-1, 1).
func DefaultJitter() float64 {
	return rand.Float64()*2 - 1 // range [-1, 1)
}

// ApplyJitter adds randomness to a delay.
// actual_delay = delay * (1 + jitterFactor * randomValue)
// where randomValue is in [-1, 1).
// jitterFactor should be in [0, 1]. If 0 or negative, returns delay unchanged.
func ApplyJitter(delay time.Duration, jitterFactor float64, rng JitterFunc) time.Duration {
	if jitterFactor <= 0 || rng == nil {
		return delay
	}
	factor := 1.0 + jitterFactor*rng()
	return time.Duration(float64(delay) * factor)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCalculateDelay(t *testing.T) {
	tests := []struct {
		name           string
		strategy       Strategy
		attempt        int
		initialDelayMs int
		maxDelayMs     int
		want           time.Duration
	}{
		// Exponential
		{"exponential attempt 0", StrategyExponential, 0, 1000, 30000, 1 * time.Second},
		{"exponential attempt 1", StrategyExponential, 1, 1000, 30000, 2 * time.Second},
		{"exponential attempt 2", StrategyExponential, 2, 1000, 30000, 4 * time.Second},
		{"exponential attempt 3", StrategyExponential, 3, 1000, 30000, 8 * time.Second},
		{"exponential capped at max_delay_ms", StrategyExponential, 10, 1000, 30000, 30 * time.Second},
		// Linear
		{"linear attempt 0", StrategyLinear, 0, 1000, 30000, 1 * time.Second},
		{"linear attempt 1", StrategyLinear, 1, 1000, 30000, 2 * time.Second},
		{"linear attempt 2", StrategyLinear, 2, 1000, 30000, 3 * time.Second},
		{"linear attempt 3", StrategyLinear, 3, 1000, 30000, 4 * time.Second},
		{"linear capped at max_delay_ms", StrategyLinear, 40, 1000, 30000, 30 * time.Second},
		// Constant
		{"constant attempt 0", StrategyConstant, 0, 2000, 30000, 2 * time.Second},
		{"constant attempt 5", StrategyConstant, 5, 2000, 30000, 2 * time.Second},
		// Edge cases
		{"default max_delay_ms when zero", StrategyExponential, 10, 1000, 0, 30 * time.Second},
		{"custom max_delay_ms", StrategyExponential, 5, 1000, 5000, 5 * time.Second},
		{"unknown strategy defaults to exponential", Strategy("unknown"), 1, 1000, 30000, 2 * time.Second},
	}
	// ...
}

func TestApplyJitter(t *testing.T) {
	tests := []struct {
		name         string
		delay        time.Duration
		jitterFactor float64
		rngValue     float64
		want         time.Duration
	}{
		{"positive jitter +10%", 1000 * time.Millisecond, 0.1, 1.0, 1100 * time.Millisecond},
		{"negative jitter -10%", 1000 * time.Millisecond, 0.1, -1.0, 900 * time.Millisecond},
		{"zero jitter factor returns unchanged", 1000 * time.Millisecond, 0.0, 0.5, 1000 * time.Millisecond},
		{"nil rng returns unchanged", 1000 * time.Millisecond, 0.1, 0, 1000 * time.Millisecond},
		{"20% jitter", 1000 * time.Millisecond, 0.2, 1.0, 1200 * time.Millisecond},
		{"50% jitter", 1000 * time.Millisecond, 0.5, -1.0, 500 * time.Millisecond},
	}
	// ...
}

func TestValidStrategy(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"exponential valid", "exponential", true},
		{"linear valid", "linear", true},
		{"constant valid", "constant", true},
		{"empty invalid", "", false},
		{"unknown invalid", "random", false},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected (new file only)

### Step 2: Extend Config to carry backoff parameters and update Resolve()
**Rationale:** Extends the data model that bridges FullConfig to ExecuteWithRetry. Small change, needed before Step 3.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/config.go` | modify | Add BackoffStrategy, MaxDelayMs, Jitter, JitterFactor to Config; update Resolve() |
| `internal/retry/merge_test.go` | modify | Add test for Resolve() transferring new fields |

#### Current Code
```go
// Config represents retry configuration parsed from YAML.
type Config struct {
	Enabled        bool `yaml:"enabled"`
	MaxAttempts    int  `yaml:"max_attempts"`
	InitialDelayMs int  `yaml:"initial_delay_ms"`
}
```

#### New Code
```go
// Config represents retry configuration parsed from YAML.
type Config struct {
	Enabled        bool    `yaml:"enabled"`
	MaxAttempts    int     `yaml:"max_attempts"`
	InitialDelayMs int     `yaml:"initial_delay_ms"`
	BackoffStrategy string `yaml:"backoff_strategy"`
	MaxDelayMs     int     `yaml:"max_delay_ms"`
	Jitter         bool    `yaml:"jitter"`
	JitterFactor   float64 `yaml:"jitter_factor"`
}
```

Updated Resolve():
```go
func (fc FullConfig) Resolve() Config {
	var c Config
	if fc.Enabled != nil {
		c.Enabled = *fc.Enabled
	}
	if fc.MaxAttempts != nil {
		c.MaxAttempts = *fc.MaxAttempts
	}
	if fc.InitialDelayMs != nil {
		c.InitialDelayMs = *fc.InitialDelayMs
	}
	if fc.BackoffStrategy != nil {
		c.BackoffStrategy = *fc.BackoffStrategy
	}
	if fc.MaxDelayMs != nil {
		c.MaxDelayMs = *fc.MaxDelayMs
	}
	if fc.Jitter != nil {
		c.Jitter = *fc.Jitter
	}
	if fc.JitterFactor != nil {
		c.JitterFactor = *fc.JitterFactor
	}
	return c
}
```

#### Tests to Write FIRST (RED phase)

```go
// Add to TestFullConfig_Resolve table
{
	name: "backoff fields transferred",
	fc: FullConfig{
		Enabled:         BoolPtr(true),
		MaxAttempts:     IntPtr(5),
		InitialDelayMs:  IntPtr(2000),
		BackoffStrategy: StringPtr("linear"),
		MaxDelayMs:      IntPtr(10000),
		Jitter:          BoolPtr(true),
		JitterFactor:    Float64Ptr(0.2),
	},
	want: Config{
		Enabled: true, MaxAttempts: 5, InitialDelayMs: 2000,
		BackoffStrategy: "linear", MaxDelayMs: 10000,
		Jitter: true, JitterFactor: 0.2,
	},
},
```

#### Impact on Existing Tests
- `TestFullConfig_Resolve` — existing test cases still pass because new fields default to zero values. Add new test case for the new fields.
- `TestResolveRetryConfig_precedence` in runner_test.go — uses `retry.Config` struct literals, but they only set `Enabled`, `MaxAttempts`, `InitialDelayMs`. With new fields defaulting to zero values, these still match (zero `BackoffStrategy` is `""`).

### Step 3: Update ExecuteWithRetry to use strategy-aware delay calculation with jitter
**Rationale:** This is the core behavioral change — now retry delays use the configured strategy, max_delay_ms cap, and jitter. Must come after Steps 1 and 2.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/retry/retry.go` | modify | Update ExecuteWithRetry to use CalculateDelay and ApplyJitter |
| `internal/retry/retry_test.go` | modify | Add tests for linear/constant/jitter behavior in ExecuteWithRetry |

#### Current Code
```go
// In ExecuteWithRetry loop:
delay := BackoffDelay(attempts-1, initialDelayMs)
```

#### New Code
```go
// In ExecuteWithRetry loop:
strategy := Strategy(cfg.BackoffStrategy)
if strategy == "" {
	strategy = StrategyExponential
}
maxDelay := cfg.MaxDelayMs
if maxDelay <= 0 {
	maxDelay = 30000
}

// ...inside loop:
delay := CalculateDelay(strategy, attempts-1, initialDelayMs, maxDelay)
if cfg.Jitter && cfg.JitterFactor > 0 {
	delay = ApplyJitter(delay, cfg.JitterFactor, DefaultJitter)
}
```

Note: The existing `BackoffDelay()` function remains for backward compatibility but is no longer called by `ExecuteWithRetry`. It could be deprecated but we'll leave it to avoid breaking external callers.

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWithRetry_linearBackoff(t *testing.T) {
	// Verify delays are 100ms, 200ms, 300ms (linear)
}

func TestExecuteWithRetry_constantBackoff(t *testing.T) {
	// Verify all delays are 200ms (constant)
}

func TestExecuteWithRetry_maxDelayMs(t *testing.T) {
	// Verify exponential delays are capped at custom max_delay_ms
}

func TestExecuteWithRetry_jitter(t *testing.T) {
	// Verify that with jitter enabled, delays are not exactly the calculated value
	// Use known seed or check range bounds
}

func TestExecuteWithRetry_exponentialDefault(t *testing.T) {
	// Verify that empty BackoffStrategy defaults to exponential
}
```

#### Impact on Existing Tests
- `TestExecuteWithRetry` existing subtests — still pass because `Config` zero value for `BackoffStrategy` is `""`, which defaults to exponential. `MaxDelayMs` 0 defaults to 30000. `Jitter` false means no jitter. So existing behavior is preserved exactly.
- `TestBackoffDelay` — unchanged, the function still exists.

### Step 4: Register custom_backoff feature gate
**Rationale:** Feature gating is the final layer. Solo tier gets exponential-only (the default), and custom strategies (linear, constant) require Professional tier. Must come after the strategy implementation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Register `custom_backoff` at TierProfessional |
| `internal/auth/registry_test.go` | modify | Test that custom_backoff requires Professional |
| `internal/auth/gate_test.go` | modify | Add gating test for custom_backoff |
| `internal/runner/runner.go` | modify | Add custom_backoff gate check when non-exponential strategy is used |
| `internal/runner/runner_test.go` | modify | Test that Solo tier with linear/constant gets exit code 6 |

#### Current Code (registry.go DefaultRegistry)
```go
r.Register(FeatureDefinition{
	Name:         "junit_xml",
	RequiredTier: TierProfessional,
	Description:  "JUnit XML output requires Professional tier ($19/month)",
	Workaround:   "Use --format json for machine-readable output, or --format tap for CI integration",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
	Name:         "junit_xml",
	RequiredTier: TierProfessional,
	Description:  "JUnit XML output requires Professional tier ($19/month)",
	Workaround:   "Use --format json for machine-readable output, or --format tap for CI integration",
})
r.Register(FeatureDefinition{
	Name:         "custom_backoff",
	RequiredTier: TierProfessional,
	Description:  "Custom backoff strategies (linear, constant) require Professional tier ($19/month)",
	Workaround:   "Use exponential backoff (the default) which is available at Solo tier",
})
return r
```

#### Runner gate check (after existing retry gate check):
```go
// Feature gate check for custom backoff strategy.
if retryCfg.Enabled && retryCfg.BackoffStrategy != "" && retryCfg.BackoffStrategy != "exponential" {
	reg := vars.Registry
	if reg == nil {
		reg = auth.DefaultRegistry()
	}
	tier := vars.Tier
	if tier == "" {
		tier = auth.TierFree
	}
	if gateErr := auth.CheckFeature(reg, "custom_backoff", tier); gateErr != nil {
		return results, requiredFailed, gateErr
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
// gate_test.go
func TestCheckFeature_customBackoff(t *testing.T) {
	registry := DefaultRegistry()
	tests := []struct {
		name    string
		tier    Tier
		wantErr bool
	}{
		{"free tier blocked", TierFree, true},
		{"solo tier blocked", TierSolo, true},
		{"professional tier allowed", TierProfessional, false},
		{"team tier allowed", TierTeam, false},
		{"enterprise tier allowed", TierEnterprise, false},
	}
	// ...
}

// registry_test.go - add to TestDefaultRegistry table
{"contains custom_backoff", "custom_backoff", true},

// runner_test.go
func TestRunRetry_customBackoffGated(t *testing.T) {
	// Solo tier with linear strategy → GateError
}

func TestRunRetry_exponentialAllowedAtSolo(t *testing.T) {
	// Solo tier with exponential strategy → no error
}
```

#### Impact on Existing Tests
- `TestDefaultRegistry` — add new entry to table, no existing entries break
- Runner tests with retry enabled at Solo tier — use default exponential, so no gate error (unchanged behavior)

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/retry/retry_test.go` | `TestBackoffDelay` | none | unchanged |
| `internal/retry/retry_test.go` | `TestExecuteWithRetry` | none | existing cases still pass (zero values default to exponential) |
| `internal/retry/merge_test.go` | `TestFullConfig_Resolve` | extend | add test case for new Config fields |
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | extend | add custom_backoff to table |
| `internal/runner/runner_test.go` | `TestResolveRetryConfig_precedence` | none | zero-value new fields match |
| `internal/runner/runner_test.go` | retry integration tests | none | use exponential (default) |

## Risks and Edge Cases
- **Risk:** Changing `Config` struct breaks runner tests that use struct literals → **Mitigation:** New fields are zero-valued, which preserves existing behavior (exponential default, no jitter, 30s cap). All existing struct literals remain valid.
- **Risk:** Jitter makes tests non-deterministic → **Mitigation:** Use injectable `JitterFunc` for deterministic testing. `ApplyJitter` takes a `rng` parameter. In `ExecuteWithRetry`, default to `DefaultJitter` but existing tests don't set `Jitter: true` so no jitter is applied.
- **Edge case:** `jitter_factor: 0` with `jitter: true` → **Handling:** `ApplyJitter` returns delay unchanged when factor <= 0.
- **Edge case:** `max_delay_ms` very small (e.g., 1ms) → **Handling:** `CalculateDelay` caps correctly; all strategies respect the cap.
- **Edge case:** Empty or unknown `backoff_strategy` → **Handling:** Defaults to exponential (backward compatible).
- **Edge case:** Negative `initial_delay_ms` → **Handling:** Existing default logic in `ExecuteWithRetry` sets it to 1000 when <= 0.
- **Risk:** `BackoffDelay()` function is still exported but no longer used internally → **Mitigation:** Keep it for backward compatibility. It still works correctly for exponential-only callers. Could be deprecated in a future task.
- **Edge case:** Solo tier with `backoff_strategy: exponential` explicitly set → **Handling:** Not gated since "exponential" is the default/allowed strategy.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Verify backoff calculations
go test ./internal/retry/... -v -run TestCalculateDelay
go test ./internal/retry/... -v -run TestApplyJitter
go test ./internal/retry/... -v -run TestExecuteWithRetry

# Verify feature gating
go test ./internal/auth/... -v -run TestCheckFeature_customBackoff
go test ./internal/runner/... -v -run TestRunRetry_customBackoffGated
```
