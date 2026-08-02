# Improvement Report: M4-010

**Task:** Backend: billing, seat management, and invitations API
**Date:** 2026-04-17
**Review:** management/reviews/M4-010-review.md
**Iteration:** 4

## Resolved Findings (Iteration 4)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Invalid tier string in `UpdateAsync` silently ignored — non-null unrecognised tier (e.g. `"quantum"`) skipped tier update and returned HTTP 200 | Added `InvalidTier` to `SubscriptionError` enum. Added guard in `UpdateAsync` before applying updates: if `tierStr` is non-null and fails `Enum.TryParse`, returns `(null, null, SubscriptionError.InvalidTier, ...)`. Added `InvalidTier` case to endpoint switch returning HTTP 400. Added `Patch_with_invalid_tier_returns_400` endpoint test. | ✓ tests pass |
| 2 | Low | `CreateAsync` in `InvitationsService` did not validate that `email` is non-empty — `""` was stored as a valid invitation | Added `InvitationError.InvalidEmail` enum value. Added guard at top of `CreateAsync`: `if (string.IsNullOrWhiteSpace(email)) return (null, null, InvitationError.InvalidEmail, ...)`. Added `InvalidEmail` case to endpoint switch returning HTTP 400. Added `Post_with_empty_email_returns_400_invalid_email` endpoint test. | ✓ tests pass |

## Previously Resolved Findings (Iteration 3)

| # | Severity | Finding | Fix Applied |
|---|----------|---------|------------|
| 1 | Low | `FreeTierSeatLimit = 1` redeclared in `SubscriptionsService` despite already existing in `OrganizationService` | Removed duplicate constant |
| 2 | Low | `Post_writes_member_invited_audit_row` did not actually verify the audit row in the DB | Updated to query `db.OrganizationAuditLog` directly |
| 3 | Low | `interval` parameter accepted any string without validation | Added guard + `InvalidInterval` enum value + endpoint handling + tests |
| 4 | Low | Audit event type emitted `"subscription.downgraded"` for interval-only changes | Fixed event-type logic; emit `"subscription.updated"` for interval-only changes |

## Previously Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied |
|---|----------|---------|------------|
| 1 | High | `ResendCooldown` error had no case in endpoint switch — fell through to HTTP 500 | Added case + endpoint test |
| 2 | Medium | Token hash literal inconsistency in SubscriptionsServiceTests | Standardized to `new string(char, 64)` pattern |
| 3 | Medium | `OrgStatus.PendingDeletion` serialized as `"pendingdeletion"` instead of `"pending_deletion"` | Introduced `ToStatusString` helper with explicit snake_case mapping |

## Previously Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied |
|---|----------|---------|------------|
| 1 | High | `ResendAsync` missing 24-hour cooldown enforcement | Added cooldown check and service-level tests |
| 2 | High | `Accept_with_expired_token` test was fake | Replaced with proper seeded-invitation test |
| 3 | High | Two planned MembersEndpoints tests missing | Added both tests |
| 4 | Medium | `DeleteOrgAsync` hardcoded FreeTierSeatLimit | Added subscription query |
| 5 | Medium | `CountSeatsAsync` duplicated in both services | Extracted into `SeatCounter` helper |
| 6 | Medium | `CheckoutRequest.Interval` not nullable | Changed to `string?` |
| 7 | Medium | `CannotRemoveOwner` returned for role-change context | Added `CannotChangeOwnerRole` enum value |
| 8 | Low | Token hash padding inconsistency in previous test | Standardized |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/` | PASS (228 tests, 0 failures) |
| Coverage (line-rate) | ~89.5% |
| Coverage (branch-rate) | ~59.9% |

## Fix Commits (Iteration 4)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| d7cbcc1 | fix(subscriptions): return 400 for invalid tier string in UpdateAsync | #1 |
| ed34ab9 | fix(invitations): reject empty email in CreateAsync with 400 invalid_email | #2 |

## Summary

2/2 findings from iteration 4 review resolved. 0 deferred. Total test count: 228 (added 2 new tests). Coverage: ~89.5% line rate.
