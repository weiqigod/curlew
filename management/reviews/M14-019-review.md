# Code Review: M14-019

**Task:** Backend: POST /webhooks/github + github_webhook_events idempotency
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-019-github-webhook-endpoint
**Iteration:** 3 (post-improve, iteration 2 findings resolved)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All expected failure modes return errors: signature mismatch → 401, missing SHA-256 header → 401, legacy SHA-1 only → 401, malformed delivery ID → 400, empty event type → 400, no secrets configured → 401, invalid JSON → 400. Dispatch exceptions caught, `RecordFailureAsync` increments attempt_count, 500 returned (or 200 on quarantine). `RecordFailureAsync` throws `InvalidOperationException` on missing row — acceptable programmer-error guard. All lifecycle methods (suspend/delete/unsuspend) return cleanly on unknown installation ID via log-and-return. |
| Input Validation | PASS | null/empty sig header → false. Missing `X-Hub-Signature-256` → 401. Non-GUID delivery ID → 400. Empty event type → 400. Empty/null secrets list → 401. Invalid JSON body after valid signature → 400. SHA-1 only → 401. All edge cases handled at the HTTP boundary. |
| Naming | PASS | No stuttering. All exported types, methods, and interfaces have doc comments. `IRerunJobQueue` is a single-method interface named for its role. `GithubWebhookPayloadParser` correctly scoped as `internal static`. Names consistent with existing Stripe counterparts. |
| Code Organization | PASS | All new files in `src/ApiTool.Backend/GitHub/Webhooks/`. `GithubWebhookCleanupHost` correctly uses `IServiceScopeFactory` to avoid captive dependency with scoped `AppDbContext`. `JsonDocument` disposed in `finally` block. `MemoryStream` in `using` block. No circular dependencies. EF InMemory / SQLite / Postgres branching clearly isolated in `GithubWebhookStore`. |
| Correctness | PASS | Raw body read before JSON parse — spec :8570 met. `CryptographicOperations.FixedTimeEquals` used for constant-time comparison. Idempotency: Postgres uses `ON CONFLICT DO NOTHING RETURNING *`; SQLite uses check-then-insert with `DbUpdateException` race guard. `ChangeTracker.Clear()` after race guard re-query. 5-failure quarantine implemented correctly. Token cache evicted after DB save in lifecycle methods — correct ordering. `IRerunJobQueue` seam injected into `GithubCheckRunHandler`; `LoggingRerunJobQueue` is default production stub. |
| Test Quality | PASS | All 10 behaviors from the task YAML covered by tests. Both iteration-2 findings resolved: `Invalid_signature_returns_401_no_row_inserted` now includes the DB assertion it names; `Valid_signature_inserts_row_returns_200` renamed to `Valid_signature_returns_200`. All test names accurately reflect their assertions. |

## Test Coverage

- Coverage: 1189 tests passed, 8 skipped, 0 failed (full suite).
- All 10 behaviors verified:
  1. HMAC-SHA256 verify + row insert: `Valid_signature_returns_200`, `Successful_processing_marks_status_processed`
  2. Multi-secret rotation: `Multi_secret_rotation_old_and_new_both_accepted`
  3. Legacy SHA-1 rejection: `Sha1_only_legacy_header_returns_401`
  4. Duplicate delivery idempotency: `Duplicate_delivery_id_returns_200_no_second_row`
  5. installation.created with null org_id: `installation_created_with_no_pending_claim_inserts_row_with_null_org_id`
  6. installation.deleted lifecycle: `installation_deleted_evicts_token_and_marks_pr_checks`, `MarkDeleted_sets_deleted_at_evicts_token_and_marks_open_pr_checks`
  7. installation.suspend lifecycle: `installation_suspend_evicts_token`, `MarkSuspended_sets_suspended_at_and_evicts_token`
  8. check_run.rerequested enqueue seam: `rerequested_with_known_external_id_enqueues_rerun_job`
  9. Handler exception quarantine: `Fifth_handler_failure_quarantines_returns_200`
  10. Raw body vs re-serialized guard: `Accepts_non_canonical_whitespace_body_signed_over_raw_bytes`

## Summary

Both Low-severity findings from iteration 2 have been correctly resolved. The test names now accurately match their assertions: `Invalid_signature_returns_401_no_row_inserted` includes the DB-level `BeNull` assertion, and `Valid_signature_inserts_row_returns_200` was renamed to `Valid_signature_returns_200`. All 10 task behaviors are covered, the Go CI gate passes, and the code meets all project standards. Ready for `/verify`.
