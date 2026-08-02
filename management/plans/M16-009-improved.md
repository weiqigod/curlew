# Improvement Report: M16-009

**Task:** Schedule executor backend endpoints with claim_token and ShardReaper extension
**Date:** 2026-05-11
**Review:** management/reviews/M16-009-review.md

## Resolved Findings — Iteration 1

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `catch (Exception)` in `Heartbeat` and `SubmitResult` handlers swallows `OperationCanceledException` | Changed both blocks to `catch (JsonException)` matching the `CoordinatorEndpoints` pattern; added `using System.Text.Json` import | ✓ tests pass |
| 2 | Medium | Three plan-specified integration tests absent: `Heartbeat_returns_402_for_free_tier_org`, `Result_returns_402_for_free_tier_org`, `NextRun_returns_403_when_user_has_no_org` | Added all three tests to `ScheduleExecutorEndpointsTests.cs`; total endpoint test count grows from 16 to 19 | ✓ tests pass |
| 3 | Low | `ClaimNextAsync_transitions_row_to_running_with_claim_token_and_deadline` used loose `BeAfter(DateTime.UtcNow)` instead of exact FakeClock assertion | Updated to `BeCloseTo(_clock.GetUtcNow().UtcDateTime.Add(ScheduleExecutorService.ClaimDeadlineDuration), TimeSpan.FromSeconds(1))` | ✓ tests pass |
| 4 | Low | Swagger test did not verify `claim_token`, `deadline`, `env_vars` schema fields | Added three assertions for `claimToken`, `deadline`, `envVars` (camelCase as ASP.NET Core OpenAPI generates) | ✓ tests pass |
| 5 | Low | `ScheduleClaimError.InternalError` is dead code with no callers | Added doc comment explaining it is intentionally reserved for future use; wildcard arm in endpoint switch already handles it | ✓ build clean |

## Resolved Findings — Iteration 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `catch (DbUpdateConcurrencyException)` block in `ClaimNextAsync` is unreachable dead code — `ScheduledRun` has no concurrency token configured, so EF will never throw that exception; the inline retry path contradicted the class-level doc comment | Removed the `try/catch` block; `SaveChangesAsync` now runs bare; class-level doc comment (which already correctly stated "last write wins, no rowversion in this slice") is now the single source of truth | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1533 passed, 8 skipped) |
| `golangci-lint run` | PASS (0 issues) |
| Go coverage | 87.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c1a821ef | fix(schedules): resolve review findings for M16-009 | Iter 1 #1, #2, #3, #4, #5 |
| e9aeb2f2 | fix(schedules): remove unreachable DbUpdateConcurrencyException catch in ClaimNextAsync | Iter 2 #1 |

## Summary

6/6 findings resolved across 2 review iterations. 0 deferred.
