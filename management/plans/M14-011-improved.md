# Improvement Report: M14-011

**Task:** Backend: Stripe webhook signature verification + stripe_webhook_events table + idempotency middleware
**Date:** 2026-05-05
**Review:** management/reviews/M14-011-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `Fifth_handler_failure_quarantines_returns_200` was a false positive — same event ID on iterations 2-5 hit the duplicate path (`Inserted=false`), never calling the dispatcher or incrementing `attempt_count`; quarantine path had zero valid HTTP coverage | Updated endpoint + store: `IdempotentInsertResult` extended with `ExistingStatus`; endpoint now re-dispatches `pending` rows (Stripe retry semantics) and only skips dispatch for `processed`/`quarantined`. Test now sends same event 5 times with throwing dispatcher, properly driving quarantine via HTTP; added DB assertion verifying `status='quarantined'` and `attempt_count=5` | ✓ tests pass |
| 2 | High | `Successful_processing_marks_status_processed_with_processed_at` only asserted HTTP 200; made no DB-state assertions | Added `AppDbContext` scope query after the HTTP call verifying `row.Status == "processed"` and `row.ProcessedAt != null` | ✓ tests pass |
| 3 | High | Boot-validation tests for tolerance=0 and empty-secrets-in-live-mode were absent (scaffolded in plan but not implemented) | Added `Boot_with_tolerance_below_minimum_throws_OptionsValidationException` and `Boot_with_empty_secrets_in_live_mode_throws_OptionsValidationException` — each builds a minimal `IHost` via `Host.CreateDefaultBuilder` with `ValidateOnStart()` and asserts `OptionsValidationException` is thrown from `StartAsync()` | ✓ tests pass |
| 4 | Medium | When secrets list is empty, endpoint fell through to `stripe_webhook_signature_invalid` log (misleading; real problem is no secrets configured) | Added early-return guard before the verification loop: `if (secrets.Count == 0) → log stripe_webhook_no_secrets_configured → return 400` | ✓ tests pass |
| 5 | Medium | No test for the "no secrets configured" path | Added `No_secrets_configured_returns_400` test: configures `Secrets=""` and asserts 400 | ✓ tests pass |
| 6 | Low | Logger injected via `ILoggerFactory` + stringly-typed name (plan artifact; review recommended `ILogger<StripeWebhookEndpoint>`) | `ILogger<T>` cannot be used with a `static class` as type argument (CS0718). Changed to `loggerFactory.CreateLogger(typeof(StripeWebhookEndpoint).FullName!)` — unambiguous, matches the class name, avoids the CS0718 error | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet test ./...` (798 passed, 8 skipped) | PASS |
| Coverage (XPlat Code Coverage line-rate) | 93.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 0a27d0a5 | fix(webhooks): resolve all review findings for M14-011 | #1, #2, #3, #4, #5, #6 |

## Design Note: Quarantine Path (Finding #1)

The review correctly identified a false positive but the proposed fix ("use unique event ID per loop iteration and prime the store") would have left the quarantine-via-HTTP path untested. The deeper issue was an architectural gap: the idempotency check treated all existing rows as "true duplicates" (returning 200 immediately), which conflicts with the retry semantics in behaviors #5-#6 (Stripe retries on 500 using the same event_id).

The correct fix was to distinguish retry semantics by existing row status:
- **`processed`** → true duplicate (already handled), skip and return 200
- **`quarantined`** → budget exhausted, break retry storm, return 200  
- **`pending`** → a previous attempt failed; re-dispatch (Stripe is retrying)

This is implemented via `IdempotentInsertResult.ExistingStatus` (new field) and a status-aware branch in the endpoint's duplicate handling. The `StripeWebhookStore.TryInsertPendingAsync` now returns the existing row's status on the `Inserted=false` path for both the Postgres (via `ExistingRowMetaAsync`) and SQLite/InMemory paths.

## Summary

6/6 findings resolved. 0 deferred. Quality gate: PASS. Coverage: 93.4%.
