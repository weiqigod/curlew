# Code Review: M6-004

**Task:** Events emitter package with NDJSON event types
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-004-events-emitter-ndjson

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned, not panicked. Wrapped with `%w`. Sentinel `ErrEmitterClosed` defined and registered in `hints_init.go`. `EmitRunError(nil)` returns an explicit error. `NewEmitter(nil)` returns an explicit error. |
| Input Validation | PASS | `NewEmitter` validates `opts.ApitestVersion != ""` (added in iter 3). Nil cliArgs normalized to `[]string{}` before marshaling (added in iter 2). Nil writer rejected. |
| Naming | PASS | No stuttering. All exported types, functions, methods, and constants have doc comments. `RequestEndInput` is clear. Short names in tight scopes. Package is `events`. |
| Code Organization | PASS | Package is isolated under `internal/output/events`, imports only `internal/errors` from the internal tree. Single responsibility. No circular dependencies. `defer e.mu.Unlock()` used throughout. No unused imports or symbols. Exported surface is minimal. |
| Correctness | PASS | `atMs` captured once in `EmitRunEnd` so `DurationMs` and `AtMs` reflect the same instant (fixed in iter 3). Atomic id counter is separate from the writer mutex — documented in struct godoc. `go test -race` passes. No goroutine leaks. |
| Test Quality | PASS | All 8 task behaviors covered. Concurrent-emit test verifies monotonic ids under `go test -race`. Golden NDJSON files validated against `docs/events-schema/v0.1.json` schema. Binary-body-over-limit truncation path covered (added in iter 3). Table-driven tests for all 5 network error kinds. `t.Run()` subtests with descriptive names. |

## Test Coverage
- Coverage: 95.7% (exceeds 80% threshold)
- Uncovered branches (all structurally unreachable / acceptable):
  - `clock()`: 66.7% — production `time.Now` path (tests always inject a deterministic clock; acceptable and documented)
  - `writeEvent()`: 81.8% — `json.Marshal` error branch (structurally unreachable for statically-typed event structs; acceptable)
  - `newRunID()`: 75.0% — `crypto/rand` failure fallback (unreachable in practice; acceptable)

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| 1. RunStart emits kind=run.start, schema_version=0.1, non-empty run_id, id=1, at_ms=0, started_at, apitest_version, cli_args | `TestEmitter_RunStart_MinimalFields` |
| 2. RequestStart+RequestEnd: monotonic ids, same run_id, same request_id, at_ms non-decreasing | `TestEmitter_RequestStartEnd_PairedIDs` |
| 3. Failed request with registered sentinel: error carries category, code, hint | `TestEmitter_RequestEnd_RegisteredSentinelHint` |
| 4. *NetworkError: category=network, code in NETWORK_* set | `TestEmitter_RequestEnd_NetworkErrorKinds` |
| 5. Concurrent goroutines: valid NDJSON, ids strictly monotonic | `TestEmitter_ConcurrentEmitMonotonicIDs` |
| 6. Body >2048 bytes: truncated content, body_size=original, body_truncated=true | `TestEmitter_BodyTruncation_OverLimit`, `TestEmitter_BodyTruncation_BinaryOverLimit` |
| 7. RunEnd event_count = total events emitted | `TestEmitter_RunEnd_EventCount` |
| 8. Every event kind validates against docs/events-schema/v0.1.json | `TestEmitter_AllKindsValidateAgainstSchema`, `TestEmitter_GoldenSchemaValidates` |

## Summary

All 10 findings from the three prior review iterations have been resolved. The package is well-structured, isolated, and correct. Coverage is 95.7%, all 8 specified behaviors have dedicated tests, and the golden NDJSON files validate against the checked-in JSON Schema. The race detector finds no data races. No new findings in this iteration.
