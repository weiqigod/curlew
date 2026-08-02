# Improvement Report: M19-001

**Task:** `if:` field on request items (CEL boolean gate)
**Date:** 2026-05-16
**Review:** management/reviews/M19-001-review.md

## Resolved Findings (Iteration 1 — 7 findings)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Missing `TestIfConditional_SensitiveValueRoutedToRuntimeSet` (behavior 4: sensitive variable value routed through `SensitiveObserver` into `summary.RuntimeSensitive`) | Added test in `internal/runner/runner_test.go`: builds a collection with `vars.token` marked sensitive, uses `if: 'vars.token == "abc"'`, asserts `summary.RuntimeSensitive.Values()` contains `"abc"` | ✓ tests pass |
| 2 | Medium | No TAP test exercising the `r.SkipReason != ""` branch in tap.go (lines 97–101) | Added `TestWriteTAP_SkipReason` to `internal/output/tap_test.go` with three sub-cases: skip+reason, skip+parent-skipped reason, and skip+empty-reason (asserting no spurious comment) | ✓ tests pass |
| 3 | Medium | No JSON test verifying `skip_reason` appears in output for skipped items | Added `TestWriteJSON_SkipReasonPresent` to `internal/output/json_test.go`: three sub-cases covering if-false reason, empty (omitted), and parent-skipped reason | ✓ tests pass |
| 4 | Medium | No JUnit test verifying `message=` attribute on `<skipped>` element carries the reason string | Added `TestWriteJUnitXML_SkipReasonInMessage` to `internal/output/junit_test.go`: asserts XML round-trips cleanly and `message="if: false"` / `message="parent skipped: ..."` appear | ✓ tests pass |
| 5 | Low | Misleading nil guard `vars.CelEvaluator != nil` in `executePhase` (runner.go:1746) — implied CelEvaluator could be nil mid-run when it cannot | Removed second conjunct; replaced with doc comment explaining the invariant: `runPhases` guarantees CelEvaluator is set whenever `collectionHasIf(col)` is true, which is the only path that sets `item.If` non-empty | ✓ tests pass |
| 6 | Low | Missing `TestIfConditional_DataDriven_SkipsAllRows` — gate fires before data-driven expansion, not per row | Added test in `internal/runner/runner_test.go`: creates a 3-row CSV, sets `if: "1 + 1 == 3"` (always false), asserts one skipped result (not three), zero exec calls, and `summary.Skipped == 1` | ✓ tests pass |
| 7 | Low | Help text not updated for `if:` and `depends_on:` as request-item fields | Added "Request Item Fields" section to `printHelpTo` in `cmd/curlew/main.go` describing `if: <CEL bool>` and `depends_on: [<name>]` with examples | ✓ build pass |

## Resolved Findings (Iteration 2 — 2 findings)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestIfConditional_NonBoolRejectedByValidate` missing from `internal/runner/runner_test.go` — task observable explicitly names this test in the runner package | Added `TestIfConditional_NonBoolRejectedByValidate` to `internal/runner/runner_test.go`; test delegates to `validator.Validate()` and asserts `ERR_CEL_TYPE` is reported for `if: "1 + 1"` | ✓ tests pass |
| 2 | Medium | `StandardActivation.Env` always `nil` at runtime — `env.<name>` references in `if:` expressions receive `no such key` errors despite being documented as accessible | Populated `act.Env` from `vars.EnvVar` in `executePhase`; added `TestIfConditional_EnvAccessible` with true-gate and false-gate sub-cases using `EnvVar: map[string]string{"RUN_MODE": "skip"}` | ✓ tests pass |

## Resolved Findings (Iteration 3 — 1 finding)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `previous` only updated on fully-successful requests (assertions pass + extraction succeeds). If a request receives an HTTP response but fails assertions, `previous` was not advanced, leaving subsequent `if:` expressions unable to inspect the actual response (e.g. `previous.status == 404` after an assertion failure). | Moved `previous = buildCelResponse(result)` to fire immediately after `execErr == nil && result != nil` — before assertion evaluation — so any executed request (regardless of assertion outcome) advances `previous`. Removed the now-redundant update at the end of the success path. Added `TestIfConditional_PreviousUpdatesOnAssertionFail` covering A (404, asserts 200 → fails), B (`if: "previous.status == 404"` → true → runs). | ✓ tests pass |

## Resolved Findings (Iteration 4 — 1 finding)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | DoD item 6 requires all six output formatters to have golden/assertion tests covering the skipped-status case. The markdown formatter correctly rendered `- skip: [name](slug.md)` at `run_md.go` line 133 but no test exercised this branch for a non-data-driven skipped request. | Added `TestMarkdown_RunMD_SkippedEntry` in `internal/output/markdown/run_md_test.go`. The test builds a report with two skipped entries (`if: false` and `parent skipped: confirm-pending-order`) and asserts the expected `- skip:` bullet lines appear, the summary table shows `Skipped == 2`, and the passing entry still appears as `- pass:`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage `internal/parser` | 90.3% |
| Coverage `internal/runner` | 85.3% |
| Coverage `internal/validator` | 92.1% |
| Coverage `internal/output` | 92.3% |
| Coverage `internal/output/markdown` | 92.6% |

All packages exceed the 80% threshold.

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `5070a219` | fix(runner): add missing if: gate tests and remove misleading nil guard | iter-1 #1, #5, #6 |
| `edaedfe3` | test(output): add SkipReason formatter tests for TAP, JSON, and JUnit | iter-1 #2, #3, #4 |
| `63f5f79d` | docs(help): add if: and depends_on: to run help text | iter-1 #7 |
| `3a78843b` | fix(runner): populate env activation surface and add missing runner test | iter-2 #1, #2 |
| `e3e96fb7` | fix(runner): update previous on any HTTP response, not only full success | iter-3 #1 |
| `9451fbe6` | fix(output/markdown): add skipped-entry test for renderRunMDEntry | iter-4 #1 |

## Summary

11/11 findings resolved across 4 improvement iterations. 0 deferred.
