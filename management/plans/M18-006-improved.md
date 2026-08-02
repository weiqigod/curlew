# Improvement Report: M18-006

**Task:** IUserAnonymiser + last-admin protection + account/data delete panel
**Date:** 2026-05-19
**Review:** management/reviews/M18-006-review.md
**Iteration:** 2

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `CancelDeletion` cascade-org reversal uses a heuristic query (`OwnerId == userId && Status == PendingDeletion`) that could erroneously restore independently-PendingDeletion orgs if a future independent code path sets orgs to PendingDeletion. | Added an explicit comment block documenting the pre-launch assumption and a `TODO(M19+)` to track replacement with a `cascade_deletion_at` column once an independent PendingDeletion code path exists. No independent code path exists today so this is safe for launch. | ✓ tests pass |
| 2 | Low | `AlreadyPending` check fires inside `DoRequestDeletionAsync` (after re-auth token consumption), violating the documented order. A user who is both already-pending AND a blocking owner receives `owner_cannot_leave` instead of `already_pending`. | Moved `AlreadyPending` check to `RequestDeletion` before `ClassifyOwnedOrgsAsync`. Pre-loaded `User` in `RequestDeletion` and passed it down to `DoRequestDeletionAsync` to avoid a redundant `FindAsync`. Updated the `reauth_consumed` test to insert a pre-consumed token directly rather than relying on a two-request sequence. Added regression test `POST_already_pending_and_blocking_returns_already_pending_not_owner_cannot_leave`. | ✓ tests pass |
| 3 | Low | Redundant `catch { await tx.RollbackAsync(ct); throw; }` — `await using var tx` already rolls back via `DisposeAsync` on exception. | Removed the `try/catch` block entirely. Rewrote the file with consistent indentation at the method body level. Updated the doc comment to document the `await using` rollback guarantee. | ✓ build passes |
| 4 | Low | `phase === 'error'` branch in `DeleteAccountPanel.svelte` was dead code — `submitDeletionRequest` set `phase = 'confirm'` (not `phase = 'error'`) on unexpected failures. | Fixed `submitDeletionRequest` to set `phase = 'error'` on unexpected non-409 failures, so the error branch now renders correctly. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `golangci-lint run` | PASS |
| Backend tests total | 2186 passed, 15 skipped, 0 failed |
| DoD filter (`UserAnonymiser\|LastAdminProtection\|AnonymisationToken\|AnonymisationSchema`) | 51 passed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `4b812e37` | fix(gdpr): remove redundant RollbackAsync + wire error phase in DeleteAccountPanel | #3, #4 |
| `7cf6953e` | fix(gdpr): move AlreadyPending before ClassifyOwnedOrgs + document cascade heuristic | #1, #2 |

## Summary

4/4 findings resolved. 0 deferred.

- The `AlreadyPending` check order fix (Finding #2) required a test update (`POST_with_consumed_reauth_token` was re-written to insert a pre-consumed token directly) and a new regression test (`POST_already_pending_and_blocking_returns_already_pending_not_owner_cannot_leave`). The fix also eliminated a redundant `FindAsync` by pre-loading `User` in `RequestDeletion`.
- The cascade heuristic (Finding #1) was addressed by documentation rather than a structural change, consistent with the review's recommendation that "if no independent `PendingDeletion` code path exists yet, document this explicitly as a known pre-launch assumption and add a TODO comment."
- Findings #3 and #4 were structural fixes with no observable behaviour change.
