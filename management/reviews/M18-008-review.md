# Code Review: M18-008

**Task:** CLI telemetry emitter: internal/telemetry package, persistent install_id, `curlew telemetry` subcommand
**Reviewer:** AI
**Date:** 2026-05-19
**Branch:** feature/M18-008-cli-telemetry-emitter
**Iteration:** 3 (post-improve, final)

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Iteration History

### Iteration 1 (original review) — FAIL, 5 findings
All resolved in improve pass 1.

| # | Finding (resolved) | Fix |
|---|--------------------|-----|
| 1 | `delete-request` POSTs with empty `install_id` | Guard `priorID != "" && statusErr == nil` before POST |
| 2 | `status` mapped all errors to "no install_id" message | `errors.Is(err, ErrNotEnabled)` check added |
| 3 | Dead exported `Store.InstallID()` method | Removed |
| 4 | Misleading `time.Sleep` in tests | Removed |
| 5 | Inconsistent `uuidRE` regex in `uuid_test.go` | Aligned with strict pattern |

### Iteration 2 (post-improve review) — FAIL, 2 findings
Both resolved in improve pass 2.

| # | Finding (resolved) | Fix Applied |
|---|--------------------|-------------|
| 1 | `Disable()` leaked `errStateMissing` sentinel to user on fresh dir (exit 1 instead of no-op) | `errors.Is(err, errStateMissing)` guard added in `Disable()` with `return nil` |
| 2 | `delete-request` printed double-space + false "event posted" on fresh dir | `actualPriorID == "" && priorID == ""` guard now prints "nothing to delete" and returns 0 |

### Iteration 3 (this review) — PASS, 0 findings

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. `ErrNotEnabled` and `errStateMissing` are correctly distinguished. Telemetry failures silently swallowed per v4-9. `Disable()` treats missing state as no-op (nil). No swallowed caller-visible errors. |
| Input Validation | PASS | `NewStore` validates empty configDir. `Enable` handles absent install_id idempotently. `DeleteLocal` handles missing files gracefully. `Client.Emit` guards nil payload. `delete-request` and `status` handle "never enabled" state correctly. |
| Naming | PASS | No stuttering. Short names in tight scopes, descriptive at package level. All exported types and functions carry doc comments. Package name lowercase single-word. No dead exported API. |
| Code Organization | PASS | `internal/backend` import forbidden by CI grep guard (verified active in `scripts/ci-local.sh:115`). `defer` used for response body close and context cancellation. Single responsibility per file. `internal/` encapsulation respected throughout. |
| Correctness | PASS | Atomic writes via tmp+rename pattern. Both files enforced mode 0600 after rename. Ring buffer capped at 10 (newest-first). 2s telemetry timeout bounded so run exit time is not delayed. Session UUID distinct from install_id. `delete-request` handles offline gracefully — local files removed regardless of POST outcome. |
| Test Quality | PASS | 29 tests in `internal/telemetry` (≥18 required). 20 tests in `cmd/curlew` telemetry files. All 9 task YAML behaviors covered. `go test -race` passes. `uuidRE` consistent across packages. New edge-case tests added in iteration 3: `TestTelemetryCmd_DeleteRequestOnFreshDir`, `TestTelemetryCmd_DisableOnFreshDir`, `TestStoreDisableOnFreshDir` directly exercise the fixed paths. |

## Test Coverage
- Coverage (`internal/telemetry`): **82.0%** (meets ≥80% requirement)
- Tests in `internal/telemetry`: **29** (meets ≥18 requirement; 33 counting subtests)
- Tests total (including `cmd/curlew` telemetry files): **49**

## Spec Compliance

All 9 behaviors from the task YAML are verified:

| Behavior | Test(s) |
|----------|---------|
| `status` with no install_id → disabled, exit 1 | `TestTelemetryCmd_StatusOnFreshDir`, smoke |
| `enable` creates install_id mode 0600, telemetry.json enabled=true | `TestTelemetryCmd_EnableCreatesFiles`, `TestStoreEnable`, `TestStoreFilePermissions` |
| POST on `run` when enabled with install_id, event_type, payload, Idempotency-Key | `TestRunFiresTelemetryWhenEnabled`, `TestClientEmit_PostsCorrectBody` |
| No POST when telemetry disabled | `TestRunDoesNotFireWhenDisabled`, `TestEmitter_DisabledNoNetwork` |
| POST failure is silent, does not surface to user | `TestRunDoesNotFailOnTelemetryError`, `TestEmitter_BackendErrorSwallowed` |
| `reset-id` regenerates install_id, old id unrecoverable | `TestTelemetryCmd_ResetID`, `TestStoreResetID` |
| `delete-request` removes files and POSTs delete event | `TestTelemetryCmd_DeleteRequest_Online`, `TestTelemetryCmd_DeleteRequest_Offline` |
| `internal/telemetry` does not import `internal/backend` | CI grep guard + `TestCoverage_EverySentinelIsRegistered` blank-import chain |
| All events carry same install_id across runs; session_uuid nested in payload | `TestRunFiresTelemetryWhenEnabled` (session_id ≠ install_id), `TestStoreEnableIdempotent` |

## Summary

All iteration-1 and iteration-2 findings were correctly resolved. The code is architecturally clean: a dedicated HTTP client with no `internal/backend` coupling (enforced by CI guard), atomic file writes with 0600 permissions, a ring-buffer emission log for GDPR export, and a bounded 2-second telemetry timeout that does not delay the runner exit. The `disable` no-op on fresh dir and the `delete-request` empty-id message are both fixed and covered by new regression tests. No new issues found in this iteration.
