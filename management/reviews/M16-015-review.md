# Code Review: M16-015

**Task:** Inbound /webhooks/gitlab handler with X-Gitlab-Token verification and idempotency
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-015-gitlab-webhook-handler
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

_No findings._

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Bare `catch` in `TryFindMatchingInstallationAsync` fixed to `catch (Exception ex) when (ex is not OperationCanceledException)` (finding #1 from iteration 1, now resolved). Dispatch `catch (Exception ex)` mirrors the established `GithubWebhookEndpoint` pattern exactly. `RecordFailureAsync` correctly throws `InvalidOperationException` with context when the row is missing. |
| Input Validation | PASS | Empty/null token, event-uuid, and event-type rejected before any DB access. Empty `WebhookSecretCiphertext` excluded via `IS NOT NULL` filter. Empty `HeadSha` excluded in pipeline handler query. Null remote IP defaults to `"unknown"`. |
| Naming | PASS | No stuttering. Interface uses `-er` suffix (`IGitLabWebhookDispatcher`). All exported types, methods, and constants have doc comments. Package names are correct. |
| Code Organization | PASS | `internal/` boundaries respected. Single responsibility per class. `JsonDocument` disposed in `finally`. DB scopes use `await using`. No unused imports or symbols. Exported surface is minimal. |
| Correctness | PASS | `DateTime.UtcNow` inconsistency in the 429 response body fixed (finding #2 from iteration 1): reads `sourceTracker.GetState(ip).QuarantinedAt` — consistent with the pre-existing quarantine path and with `FakeClock` under test. Constant-time comparison and full-iteration design are correct. Idempotency, per-IP quarantine, and per-event-row quarantine are all correctly implemented. |
| Test Quality | PASS | `Pipeline_hook_invokes_dispatcher_with_correct_envelope` endpoint integration test added (finding #3 from iteration 1). All 8 task behaviors are covered by at least one test. 50+ tests across 7 new test files: 8 tracker unit tests, 8 store unit tests, 4 dispatcher unit tests, 7 pipeline handler tests, 5 payload parser tests, 3 cleanup tests, and 17 endpoint integration tests. Assertions are specific; table-driven patterns used where applicable. Test doubles (`NoopGitLabWebhookDispatcher`, `ThrowingGitLabWebhookDispatcher`, `RecordingGitLabWebhookDispatcher`) are well-designed. |

## Test Coverage
- Coverage: Go gate passes (no Go files changed in this task). C# backend coverage not measured in the `ci-local.sh --go` gate.
- Backend test count: ~51 new tests across 7 new test files. All behaviors from the task YAML covered.
- No missing coverage identified.

## Findings from Previous Iteration (all resolved)

| # | Severity | Finding | Resolution |
|---|----------|---------|-----------|
| 1 | High | Bare `catch` in `TryFindMatchingInstallationAsync` swallowed `OperationCanceledException` | Fixed: `catch (Exception ex) when (ex is not OperationCanceledException)` |
| 2 | Medium | `DateTime.UtcNow` in new-quarantine 429 response inconsistent with tracker's `TimeProvider` | Fixed: reads back `sourceTracker.GetState(ip).QuarantinedAt` |
| 3 | Low | Missing endpoint-level Pipeline Hook dispatcher invocation test | Fixed: `Pipeline_hook_invokes_dispatcher_with_correct_envelope` added |

## Summary
All three findings from iteration 1 are resolved. The implementation is architecturally sound, mirrors the established GitHub/Stripe webhook patterns, and correctly implements constant-time token comparison, full-iteration (no timing leak on match), idempotency, per-IP source quarantine, per-event-row dispatcher-failure quarantine, and daily retention cleanup. Spec compliance is complete: all 8 task behaviors are tested, documentation of single-secret-only support is in `docs/SPECIFICATION.md:9259` and the endpoint doc comment, and CHANGELOG is updated.
