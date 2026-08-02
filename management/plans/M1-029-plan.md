# Implementation Plan: M1-029

## Overview
Add a 1,000-request guard rail to the execution pipeline. When a collection exceeds the limit, execution stops with exit code 2 and a message suggesting to split into smaller collections.

## Task Details
- **ID:** M1-029
- **Title:** Guard rail (1,000-request limit)
- **Phase:** M1: Core CLI
- **Priority:** 29
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-003 | HTTP execution | done |

## Implementation Steps

### Step 1: Add guard rail constant, Summary fields, and counter to executePhase

**Rationale:** The counter and Summary fields are the foundation; all other changes depend on them. Smallest blast radius — only touches internal runner types and logic.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `MaxRequests` var, `LimitExceeded`/`RequestsExecuted` fields to Summary, add counter param to `executePhase`, wire counter in `Run()` |
| `internal/runner/runner_test.go` | modify | Add guard rail test cases |

#### Current Code

```go
// Summary holds aggregate execution results.
type Summary struct {
	Total                   int
	Passed                  int
	Failed                  int
	Skipped                 int
	AssertionFailures       int
	TeardownErrors          int
	TeardownAssertionErrors int
	Duration                time.Duration
}
```

```go
func executePhase(
	ctx context.Context,
	items []parser.RequestItem,
	scope *variable.Scope,
	exec ExecuteFunc,
	vars VarSources,
	phase Phase,
	checkRequired bool,
	stopOnFailure bool,
) ([]RequestResult, bool, error) {
```

```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, vars VarSources) ([]RequestResult, *Summary, error) {
	// ...
	setupResults, reqFailed, fatalErr := executePhase(ctx, col.Setup, scope, exec, vars, PhaseSetup, true, false)
	// ...
	mainResults, _, fatalErr := executePhase(ctx, col.Requests, scope, exec, vars, PhaseMain, false, col.Options.StopOnFailure)
	// ...
	tdResults, _, fatalErr := executePhase(ctx, col.Teardown, scope, exec, vars, PhaseTeardown, false, false)
```

#### New Code

```go
// MaxRequests is the guard rail limit for abuse prevention.
// Override in tests or via ldflags: -ldflags "-X '...runner.MaxRequests=2000'"
var MaxRequests = 1000

// Summary holds aggregate execution results.
type Summary struct {
	Total                   int
	Passed                  int
	Failed                  int
	Skipped                 int
	AssertionFailures       int
	TeardownErrors          int
	TeardownAssertionErrors int
	Duration                time.Duration
	LimitExceeded           bool // true when guard rail stopped execution
	RequestsExecuted        int  // HTTP requests actually sent (not skipped)
}
```

```go
func executePhase(
	ctx context.Context,
	items []parser.RequestItem,
	scope *variable.Scope,
	exec ExecuteFunc,
	vars VarSources,
	phase Phase,
	checkRequired bool,
	stopOnFailure bool,
	counter *int,
	maxRequests int,
) ([]RequestResult, bool, error) {
	// ... existing preamble ...
	for _, item := range items {
		if stopped {
			// ... existing skip logic ...
			continue
		}
		// NEW: guard rail check before executing
		if *counter >= maxRequests {
			results = append(results, RequestResult{Name: item.Name, Phase: phase, Skipped: true})
			stopped = true
			continue
		}
		// ... existing context check, interpolation, exec call ...
		// After exec() call (regardless of pass/fail/error):
		*counter++
		// ... rest of existing logic ...
	}
```

```go
func Run(...) (...) {
	// ...
	counter := 0

	// Phase 1: Setup
	setupResults, reqFailed, fatalErr := executePhase(ctx, col.Setup, scope, exec, vars, PhaseSetup, true, false, &counter, MaxRequests)
	// ...

	// Phase 2: Main
	mainResults, _, fatalErr := executePhase(ctx, col.Requests, scope, exec, vars, PhaseMain, false, col.Options.StopOnFailure, &counter, MaxRequests)
	// ...

	// Phase 3: Teardown
	tdResults, _, fatalErr := executePhase(ctx, col.Teardown, scope, exec, vars, PhaseTeardown, false, false, &counter, MaxRequests)
	// ...

	summary.RequestsExecuted = counter
	if counter >= MaxRequests {
		summary.LimitExceeded = true
	}
	// ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_GuardRail(t *testing.T) {
	tests := []struct {
		name             string
		collection       *parser.Collection
		maxRequests      int
		wantExecuted     int
		wantLimitExceed  bool
		wantPassed       int
		wantSkipped      int
	}{
		{"999 requests all succeed", makeCollection(generateNames(999), false), 1000, 999, false, 999, 0},
		{"exactly 1000 requests all succeed", makeCollection(generateNames(1000), false), 1000, 1000, false, 1000, 0},
		{"1001 requests stops at 1000", makeCollection(generateNames(1001), false), 1000, 1000, true, 1000, 1},
		{"limit hit across setup main teardown", collectionWithPhases(500, 400, 200), 1000, 1000, true, 1000, 100},
		{"limit hit during setup skips main", collectionWithPhases(1001, 5, 0), 1000, 1000, true, 1000, 6},
		{"small limit for easy testing", makeCollection(generateNames(6), false), 5, 5, true, 5, 1},
		{"skipped by stop_on_failure do not count toward limit", /* stop_on_failure collection */, 1000, ...},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := MaxRequests
			MaxRequests = tt.maxRequests
			t.Cleanup(func() { MaxRequests = old })

			results, summary, err := Run(context.Background(), tt.collection, successExecutor, VarSources{})
			// assert err == nil
			// assert summary.RequestsExecuted == tt.wantExecuted
			// assert summary.LimitExceeded == tt.wantLimitExceed
			// assert summary.Passed == tt.wantPassed
			// assert summary.Skipped == tt.wantSkipped
		})
	}
}
```

#### Impact on Existing Tests
- All existing `TestRun` tests pass unchanged — they use collections of 1-5 requests, well under 1000
- `executePhase` signature change is internal to the package; tests call `Run()` not `executePhase` directly

### Step 2: Add guard rail output formatting

**Rationale:** Output methods must exist before the CLI can use them. Isolated to the output package.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `GuardRail()` printer method |
| `internal/output/json.go` | modify | Add `GuardRailJSON` struct and field on `JSONOutput` |
| `internal/output/terminal_test.go` | modify | Test `GuardRail()` output |
| `internal/output/json_test.go` | modify | Test `GuardRailJSON` serialization |

#### New Code — Terminal

```go
// GuardRail writes the guard rail limit exceeded message.
func (p *Printer) GuardRail(executed, limit int) {
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize("✗ Request limit exceeded", ansiRed, p.color))
	_, _ = fmt.Fprintf(p.w, "  Executed %d requests (limit: %d)\n", executed, limit)
	_, _ = fmt.Fprintf(p.w, "  %s\n", colorize("Hint: Split this collection into multiple smaller collections", ansiGray, p.color))
}
```

#### New Code — JSON

```go
// GuardRailJSON represents the guard rail limit in JSON output.
type GuardRailJSON struct {
	LimitExceeded    bool   `json:"limit_exceeded"`
	RequestsExecuted int    `json:"requests_executed"`
	Limit            int    `json:"limit"`
	Message          string `json:"message"`
}

// JSONOutput additions:
type JSONOutput struct {
	// ... existing fields ...
	GuardRail *GuardRailJSON `json:"guard_rail,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_GuardRail(t *testing.T) {
	tests := []struct {
		name     string
		executed int
		limit    int
		color    bool
		wantStr  string
	}{
		{"plain text", 1000, 1000, false, "Request limit exceeded"},
		{"includes hint", 1000, 1000, false, "Split this collection"},
		{"shows count", 500, 500, false, "Executed 500 requests (limit: 500)"},
	}
}

func TestGuardRailJSON_Serialization(t *testing.T) {
	// Verify GuardRailJSON marshals correctly
	// Verify omitempty on JSONOutput.GuardRail
}
```

#### Impact on Existing Tests
- No existing tests affected — new methods and types only
- `JSONOutput` gains an `omitempty` field that is nil by default, so existing JSON tests produce identical output

### Step 3: Wire guard rail exit code 2 in runCmd

**Rationale:** Final integration step — connects runner Summary.LimitExceeded to exit code 2 and output formatting for all three formats (terminal, JSON, TAP).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Check `summary.LimitExceeded` and return exit code 2 with format-appropriate output |
| `cmd/apitest/main_test.go` | modify | Integration tests for exit code 2 across formats |

#### Current Code (terminal exit logic, lines 388-399)

```go
out.SummaryWithDuration(summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Duration)

// Teardown failures are informational and do not affect exit code
mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
mainFailed := summary.Failed - summary.TeardownErrors
if mainAssertionFailed > 0 {
	return 1
}
if mainFailed > 0 {
	return 4
}
return 0
```

#### New Code (terminal exit logic — guard rail check after summary)

```go
out.SummaryWithDuration(summary.Total, summary.Passed, summary.Failed, summary.Skipped, summary.Duration)

// Guard rail takes precedence — execution was incomplete
if summary.LimitExceeded {
	out.GuardRail(summary.RequestsExecuted, runner.MaxRequests)
	return 2
}

// Teardown failures are informational and do not affect exit code
// ... existing exit code logic unchanged ...
```

For JSON format (after `buildJSONOutput` call):

```go
if summary != nil && summary.LimitExceeded {
	jsonOut.Status = "guard_rail"
	jsonOut.GuardRail = &output.GuardRailJSON{
		LimitExceeded:    true,
		RequestsExecuted: summary.RequestsExecuted,
		Limit:            runner.MaxRequests,
		Message:          "Request limit exceeded. Split this collection into multiple smaller collections.",
	}
	_ = output.WriteJSON(os.Stdout, jsonOut)
	return 2
}
```

For TAP format (after `WriteTAP` call):

```go
if summary != nil && summary.LimitExceeded {
	_, _ = fmt.Fprintf(os.Stdout, "# Guard rail: executed %d requests (limit: %d)\n",
		summary.RequestsExecuted, runner.MaxRequests)
	return 2
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_GuardRail(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantExit   int
		wantOutput string // substring to check
	}{
		{"terminal exit code 2", "", 2, "Request limit exceeded"},
		{"terminal hint message", "", 2, "Split this collection"},
		{"json status guard_rail", "json", 2, "guard_rail"},
		{"json has guard_rail field", "json", 2, "limit_exceeded"},
		{"tap comment", "tap", 2, "# Guard rail"},
	}
}
```

Note: Integration tests need a test HTTP server and a collection that exceeds a lowered `MaxRequests`. Override `runner.MaxRequests` to a small value (e.g., 3) in tests and create a 4-request collection.

#### Impact on Existing Tests
- No existing tests affected — guard rail check is a new code path that only triggers when `summary.LimitExceeded == true`
- All existing tests use small collections, so `LimitExceeded` is always `false`

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | `TestRun` | none | no changes needed |
| `internal/runner/runner_test.go` | `TestRun_GuardRail` | new | write new test cases |
| `internal/output/terminal_test.go` | `TestPrinter_GuardRail` | new | write new test cases |
| `internal/output/json_test.go` | `TestGuardRailJSON` | new | write new test cases |
| `cmd/apitest/main_test.go` | `TestRunCmd_GuardRail` | new | write integration tests |

## Risks and Edge Cases

- **Risk:** Exactly 1000 requests — boundary condition → **Mitigation:** Counter check is `*counter >= maxRequests` before exec. The 1000th request executes (counter goes 999→1000), the 1001st is blocked. Explicit test case for this boundary.
- **Edge case:** Limit hit during setup → **Handling:** Main requests are skipped. Teardown requests that would exceed the limit are also skipped. This is correct — the guard rail applies to total HTTP load uniformly.
- **Edge case:** `stop_on_failure` stops before limit → **Handling:** No guard rail error. Requests skipped by `stop_on_failure` don't increment the counter, which is correct.
- **Edge case:** Limit hit during teardown → **Handling:** Partial teardown results are in the summary. Remaining teardown requests are skipped.
- **Risk:** `MaxRequests` as `var` leaking between tests → **Mitigation:** Tests save/restore via `t.Cleanup()`.
- **Edge case:** Exit code precedence → **Handling:** Exit code 2 (guard rail) takes precedence over 1 (assertion failure) and 4 (network error) because the execution was incomplete. If results are partial, the guard rail exit code is more informative.
- **Edge case:** Context cancellation + guard rail → **Handling:** Context cancellation sets `stopped = true` via existing logic, preventing the guard rail check from being reached for subsequent items. The counter reflects only requests that were actually sent.
- **Risk:** JSON backward compatibility → **Mitigation:** `GuardRail` field is `omitempty`, so it only appears when limit is exceeded. Existing consumers unaffected.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Use test infrastructure (override MaxRequests in integration test) to create
# a collection exceeding the limit, then verify:
# - Exit code is 2
# - Message suggests splitting collections
# - Summary includes requests completed before the limit
# - All three output formats handle the guard rail correctly
```
