# Code Review: M16-009

**Task:** Schedule executor backend endpoints with claim_token and ShardReaper extension
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-009-schedule-executor-endpoints
**Iteration:** 3 (post-improvement, final)

## Verdict: PASS

## Findings

No findings.

## Previous Findings Status

All findings from the previous two review iterations are resolved:

| Iter | # | Severity | Finding | Status |
|------|---|----------|---------|--------|
| 1 | 1 | Medium | `catch (Exception)` swallowing `OperationCanceledException` in Heartbeat and SubmitResult handlers | Fixed |
| 1 | 2 | Medium | Three missing integration tests | Fixed |
| 1 | 3 | Low | Deadline assertion loose instead of exact FakeClock assertion | Fixed |
| 1 | 4 | Low | Swagger test missing schema field assertions | Fixed |
| 1 | 5 | Low | `InternalError` dead code undocumented | Fixed |
| 2 | 1 | Low | `catch (DbUpdateConcurrencyException)` in `ClaimNextAsync` was unreachable dead code (no concurrency token configured) | Fixed — `try/catch` removed; `SaveChangesAsync` runs bare; class-level doc comment is the single source of truth |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All service-level error paths correct; endpoint `catch (JsonException)` pattern matches codebase convention; no swallowed errors; errors wrapped with `fmt.Errorf`-equivalent context |
| Input Validation | PASS | Null body, malformed run ID, missing/invalid claim_token all handled with appropriate status codes |
| Naming | PASS | No stuttering; doc comments on all exported symbols; `ScheduleClaimError` correctly distinguished from `ScheduleExecutorError` (tier gate); consistent with project conventions |
| Code Organization | PASS | `ScheduleExecutorService` correctly separated from `SchedulesService`; `CoordinatorService` reaper extension is additive; `internal/` package boundaries respected; `Program.cs` registration at the correct DI scope |
| Correctness | PASS | `DbUpdateConcurrencyException` dead code removed; `HasMaxLength(20)` covers all enum values including `Reaped` and `Skipped`; `OnDelete(SetNull)` correctly configured for nullable `ResultId` FK; stack-up prevention correctly uses `HashSet` for O(1) lookup |
| Test Quality | PASS | All 9 spec behaviors covered; `ClaimReaped` vs `StaleClaim` paths tested separately; `ReapStaleShards_sums_shard_and_scheduled_run_counts` verifies additive reaper count; deadline asserted with FakeClock `BeCloseTo`; `SubmitResultAsync_returns_InvalidRequest_when_claim_token_missing_or_malformed` is table-driven with 3 cases |

## Test Coverage
- Go coverage: N/A (C# backend task; Go gate ran clean — `ci-local.sh --go` all PASS)
- .NET coverage: Not measured in `--go` gate; measured under `--full` in `/verify`
- Behavior coverage: All 9 behaviors from task YAML covered by at least one test

## Summary

The implementation is correct and complete. All six findings across two prior review iterations are fully resolved. The `DbUpdateConcurrencyException` dead code from iteration 2 has been removed — `ClaimNextAsync` now runs `SaveChangesAsync` bare, consistent with the class-level doc comment. ShardReaper extension, stack-up prevention, tier gating, DTO shapes, migration (applies and rolls back cleanly), and test coverage are all solid. No new issues found.
