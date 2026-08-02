# Improvement Report: M18-008

**Task:** CLI telemetry emitter: internal/telemetry package, persistent install_id, `curlew telemetry` subcommand
**Date:** 2026-05-19
**Review:** management/reviews/M18-008-review.md
**Iteration:** 2 (post second review)

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `Disable()` returns `errStateMissing` sentinel wrapped as a user-facing error when `telemetry.json` is absent (telemetry never enabled). User sees `"telemetry: disable: telemetry: state file absent"` and exits 1. | Added `errors.Is(err, errStateMissing)` guard in `store.go Disable()` — returns `nil` immediately (already disabled, no-op). Added `TestStoreDisableOnFreshDir`. | ✓ tests pass |
| 2 | Medium | `telemetryDeleteRequest` prints `"telemetry: install_id  deleted locally; delete-request event posted"` (double space, empty install_id, false "event posted" claim) when run before `enable`. | Added early check: if both `actualPriorID` and `priorID` are empty after `DeleteLocal()`, print `"telemetry: nothing to delete (telemetry was never enabled)"` and return 0. Added `TestTelemetryCmd_DeleteRequestOnFreshDir` and `TestTelemetryCmd_DisableOnFreshDir`. | ✓ tests pass |

## Previously Resolved Findings (Iteration 1)

| # | Finding (resolved) | Fix |
|---|--------------------|-----|
| 1 | `delete-request` POSTs with empty `install_id` | Guard `priorID != "" && statusErr == nil` before POST ✓ |
| 2 | `status` mapped all errors to "no install_id" message | `errors.Is(err, ErrNotEnabled)` check added ✓ |
| 3 | Dead exported `Store.InstallID()` method | Removed ✓ |
| 4 | Misleading `time.Sleep` in tests | Removed ✓ |
| 5 | Inconsistent `uuidRE` regex in `uuid_test.go` | Aligned with strict pattern ✓ |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/telemetry`) | 82.0% |

## Fix Commits (Iteration 2)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `8fa8a4bf` | fix(telemetry): Disable() is a no-op when telemetry.json absent | #1 |
| `90bed1a3` | fix(telemetry): delete-request on fresh dir shows 'nothing to delete' | #2 |

## Summary
2/2 iteration-2 findings resolved. 0 deferred. All 7 total findings across both iterations resolved.
