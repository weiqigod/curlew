# Improvement Report: M16-015

**Task:** Inbound /webhooks/gitlab handler with X-Gitlab-Token verification and idempotency
**Date:** 2026-05-11
**Review:** management/reviews/M16-015-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Bare `catch` in `TryFindMatchingInstallationAsync` swallows `OperationCanceledException` — client disconnect during KMS decrypt caused silent 401 instead of propagating cancellation. | Changed to `catch (Exception ex) when (ex is not OperationCanceledException)` with a comment explaining that `OperationCanceledException` is intentionally re-thrown. | ✓ tests pass |
| 2 | Medium | `DateTime.UtcNow` used in the new-quarantine 429 response body instead of the tracker's `QuarantinedAt` timestamp, making the two 429 paths inconsistent and breaking `FakeClock` under test. | After `sourceTracker.RecordFailure(ip)`, reads back `sourceTracker.GetState(ip).QuarantinedAt` and uses that value in the JSON response — same pattern as the pre-existing quarantine path. | ✓ tests pass |
| 3 | Low | Endpoint-level test for behavior #4 (Pipeline Hook → dispatcher invoked) missing. Only unit-level coverage existed; the wire path (endpoint → dispatcher) was not tested. | Added `Pipeline_hook_invokes_dispatcher_with_correct_envelope` test using a new `RecordingGitLabWebhookDispatcher` that captures envelopes. Asserts dispatcher is called once with correct `EventType` and `EventUuid`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS |
| `dotnet test ApiTool.Backend.sln` | PASS (1689 passed, 8 skipped) |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Backend test count | 1689 passed (45 GitLab webhook tests) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f863e497 | fix(gitlab-webhook): fix bare catch, DateTime.UtcNow, and add dispatcher coverage | #1, #2, #3 |

## Summary
3/3 findings resolved. 0 deferred.
