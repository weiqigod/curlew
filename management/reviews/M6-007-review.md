# Code Review: M6-007

**Task:** Validation harness: gate for v0.1 → v1.0 schema promotion
**Reviewer:** AI
**Date:** 2026-04-22
**Branch:** feature/M6-007-validation-harness
**Iteration:** 3 (post-improve, iteration-2 finding resolved)

## Verdict: PASS

## Findings

No findings. All previous findings from iterations 1 and 2 are resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. No swallowed errors. Sentinels `ErrAssertionFailed`, `ErrUndefinedVariable` used correctly. `enrichInterpErr` mutates via `errors.As` safely. `RegisterTypeClassifier` correctly adds `*GateError` classification. |
| Input Validation | PASS | `ParseExpectation` handles malformed YAML. `getNestedValue` handles missing paths. `matchEvent` handles empty stream. `findCollectionFile` handles missing collection with `t.Fatalf`. |
| Naming | PASS | No stuttering. All exported types (`Expectation`, `ExpectedEvent`, `Scenario`, `ParseExpectation`) have doc comments. Package names correct. Helper names are descriptive. |
| Code Organization | PASS | `cmd/curlew-agent-harness/` correctly scoped as test-only. `harness.go` uses `//go:build never` to exclude from release builds. `contract.go` in `package harness_test` coexists with `harness.go` in `package harness` cleanly. `internal/` boundaries respected. No circular dependencies. |
| Correctness | PASS | All 7 harness scenarios pass (auth-missing, bad-yaml, circular-include, failing-assertion, feature-gate-denied, missing-variable, unreachable-host). Schema version consistently "1.0" across golden files, JSON schema, `events.go`, and docs. `enrichInterpErr` correctly propagates FilePath/Line. Sentinel-first priority in `LookupHint` is correct and tested. `run_failed_assertion.ndjson` golden correctly omits error block (emitter tested independently of adapter; full-stack tested by `TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd`). |
| Test Quality | PASS | All iteration-2 finding resolved: `toFloat64` `int64` branch now covered by synthetic `int64(200)` test case; coverage is 80.0% (the only uncovered line is the defensive `return 0, false` which is unreachable in practice). Negative tests present (`TestHarness_FailsLoudlyOnContractBreach`, `TestHarness_ContractBreach_ExitCodeMismatch`, `TestHarness_ContractBreach_HintVerb`). All 7 task behaviors covered. |

## Test Coverage

- `cmd/curlew-agent-harness`: **93.9%** — above 80% threshold
- `internal/variable`: **96.7%** — above threshold
- `internal/runner`: **83.6%** — above threshold
- `internal/errors`: **93.5%** — above threshold
- `internal/auth`: **89.6%** — above threshold
- `internal/output/events`: **95.7%** — above threshold
- `cmd/curlew`: **80.3%** — at threshold
- All packages above or at 80% threshold.

## Summary

All 7 harness scenarios pass cleanly. The iteration-2 residual finding (untested `int64` branch in `toFloat64`) is now resolved with a targeted test case that achieves 80% function coverage. The implementation is architecturally sound: the harness correctly validates agent-diagnosability contracts for every failure category, the schema promotion to v1.0 is complete and internally consistent, and the production-code fixes (VAR_UNDEFINED category to input, GateError type classifier, assertion sentinel on request.end, file/line enrichment in enrichInterpErr) are all correct, well-tested, and integrated cleanly. The pre-audit CI gate passed with zero lint issues and all smoke tests green.
