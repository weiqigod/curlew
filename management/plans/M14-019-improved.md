# Improvement Report: M14-019

**Task:** Backend: POST /webhooks/github + github_webhook_events idempotency
**Date:** 2026-05-06
**Review:** management/reviews/M14-019-review.md
**Iteration:** 2 (post-second-review)

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `Invalid_signature_returns_401_no_row_inserted` — test name claimed "no_row_inserted" but body only asserted HTTP 401, no DB assertion. | Added `FirstOrDefaultAsync` lookup via `_factory.Services.CreateScope()` and `BeNull("signature rejection must not insert a row")` assertion. | ✓ tests pass |
| 2 | Low | `Valid_signature_inserts_row_returns_200` — test name claimed "inserts_row" but body only asserted HTTP 200. DB insertion already covered by `Successful_processing_marks_status_processed`. | Renamed test to `Valid_signature_returns_200` to remove the false contract. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Coverage | 1189 passed, 8 skipped, 0 failed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 532839d0 | fix(tests): fix misleading test names in GithubWebhookEndpointTests | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
