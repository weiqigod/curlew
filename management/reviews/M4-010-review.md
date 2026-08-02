# Code Review: M4-010

**Task:** Backend: billing, seat management, and invitations API
**Reviewer:** AI
**Date:** 2026-04-17
**Branch:** feature/M4-010-billing-seats-invitations

## Verdict: PASS

## Findings

No findings. All previously-raised issues (iteration 3: interval validation, audit event type,
ResendCooldown missing case, PendingDeletion serialization; iteration 4: silent invalid-tier
ignore in UpdateAsync, missing empty-email guard in InvitationsService.CreateAsync) have been
confirmed resolved in the current code.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All service error codes surface to HTTP via complete switch expressions. No swallowed errors. All enum values in SubscriptionError, InvitationError, and MemberError are handled in their respective endpoint switch arms. Exceptional states fall through to HTTP 500. |
| Input Validation | PASS | Tier validated with guard returning SubscriptionError.InvalidTier before DB access in UpdateAsync (lines 118–123). Empty email guarded in InvitationsService.CreateAsync (lines 29–30) returning InvitationError.InvalidEmail → HTTP 400. Interval validated in both CreateCheckoutAsync and UpdateAsync. Role validated (no owner role allowed in invitations). OrgId, SubscriptionId, InvitationId all validated at endpoint boundary. |
| Naming | PASS | No stuttering. All exported types, methods, and properties have doc comments. IStripeGateway follows the -I prefix interface convention; FakeStripeGateway and StripeGateway are correctly named. SeatCounter is a clear, non-stuttering static helper class. |
| Code Organization | PASS | Clean service/endpoint/DTO/error separation per domain. SeatCounter shared helper prevents drift between SubscriptionsService and InvitationsService. No circular dependencies. internal/ boundary respected. |
| Correctness | PASS | Seat-count rule correctly enforced: members + pending invitations (not accepted, not revoked, not expired). Downgrade-below-active-seat-count check present. Invitation token stored as SHA-256 hash only; raw token returned once in response. Resend cooldown (24 h) and limit (3 resends, SendCount > MaxResends where MaxResends=3) both enforced and tested. AlreadyMember case on accept marks invitation accepted to avoid stuck rows. Rate limiter policies registered for all 7 new policy names in both live and no-op (Testing) branches. |
| Test Quality | PASS | All 10 task behaviors have dedicated tests. Audit row assertion verified via direct DB query. Edge cases covered: empty email, invalid tier, invalid interval, expired token, resend cooldown/limit, seat limit, duplicate pending invitation. Table-driven tests used for SubscriptionIdTests. 228 total tests, 0 failures. |

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Checkout with tier=team seat_count=5 returns checkout_url and 200 | `Checkout_returns_200_with_checkout_url_for_owner` |
| 2 | GET /subscriptions returns null subscription and tier=free when no sub | `Get_returns_null_subscription_and_tier_free_when_no_sub` |
| 3 | PATCH increases seat_count and returns proration block | `Patch_increases_seats_and_returns_proration_block` |
| 4 | Downgrade attempt below active members returns 403 subscription_downgrade_blocked | `Patch_with_downgrade_below_active_members_returns_403_downgrade_blocked` |
| 5 | Admin POST invitation creates 201 and writes member.invited audit row | `Post_creates_invitation_and_returns_201_with_email_and_role`, `Post_writes_member_invited_audit_row` |
| 6 | Duplicate invitation while pending returns 409 invitation_pending | `Post_duplicate_email_while_pending_returns_409_invitation_pending` |
| 7 | Accept with valid token creates member and marks accepted | `Accept_with_valid_token_creates_member_and_marks_accepted` |
| 8 | Accept with expired token returns 410 invitation_expired | `Accept_with_expired_token_returns_410_invitation_expired` |
| 9 | Resend beyond 3 sends returns 429 resend_limit_reached | `Resend_beyond_3_sends_returns_429_resend_limit_reached` |
| 10 | Swagger lists all subscription, invitation, and member endpoints | `Swagger_lists_all_subscription_invitation_and_member_endpoints` |

## Test Coverage
- Total tests: 228 (0 failures)
- Tests matching M4-010 filter (Subscriptions|Invitations|Seats|Members): 59
- Coverage: ~89.5% line rate, ~58.3% branch rate (unchanged from iteration 4)
- Missing coverage: none identified — all behaviors and edge cases from the task YAML are exercised

## Summary

The implementation is complete and correct. All findings from iterations 3 and 4 have been
addressed: invalid tier in UpdateAsync now returns a proper 400/InvalidTier response, and
empty email in InvitationsService.CreateAsync is guarded with a dedicated InvalidEmail error
code. All 10 task behaviors are covered by tests, the EF migration is in place, Swagger lists
all 15 required endpoints, seat counting is enforced via a shared SeatCounter helper, and the
IStripeGateway abstraction has both FakeStripeGateway and StripeGateway stub implementations.
All 228 tests pass with 0 failures.
