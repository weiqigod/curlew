# Code Review: M18-004

**Task:** GDPR export endpoint: queue + signed-URL + account/data web page
**Reviewer:** AI
**Date:** 2026-05-18
**Branch:** feature/M18-004-user-data-export
**Iteration:** 4 (post-improve iteration 3)

## Verdict: PASS

## Findings

No findings. The single Low-severity finding from iteration 3 (tautological footer test) has been
correctly resolved: `account-data-export.spec.ts` now imports `+page.svelte` via `?raw` and
asserts the href string is present in the actual template source. The test can now fail if the
footer link is removed or changed.

No new findings were introduced.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `DbUpdateConcurrencyException` in both the `Ready → Expired` GET path and the builder claim-transition path is correctly caught and handled. `OperationCanceledException` handled in `ExecuteAsync`. Builder failure path stores exception type only (PII mitigation correct). `ArgumentNullException.ThrowIfNull` guards on all public-facing entry points. The `catch(Exception)` in the signed-URL generation path is documented as best-effort and the CancellationToken provides a safety valve for the pipe task under store failures. |
| Input Validation | PASS | Null userId → 401, cross-user enumeration blocked by scoped query. All `IObjectStore` implementations validate key/content nullability. `GdprAttributeScanner.Scan` validates assembly null. `UserExportBundleAssembler.AssembleAsync` validates db and manifest. |
| Naming | PASS | `s_jsonOpts` naming correct. All exported symbols carry doc comments. No stuttering. `IObjectStore` interface follows single-responsibility. Package names correct. `UserExportBuilderHost`, `UserExportBundleAssembler`, `UserDataExportEndpoints`, and `UserExportRequestDto` are unambiguous. |
| Code Organization | PASS | Backend separation is clean: entity, migration, object-store abstraction, assembler, host, and endpoints each own a distinct domain. `InMemoryObjectStore` and its test endpoint are correctly gated to Dev+Testing. `InternalRunExportBuilderEndpoint` is guarded by `InternalAccessFilter`. `UserExportBuilderHost` is not registered in Testing environment. No circular dependencies. `IAsyncDisposable` used for `S3ObjectStore` and `GcsObjectStore`. |
| Correctness | PASS | Optimistic concurrency token (`Version`) present and tested. `Two_concurrent_ticks_only_process_each_row_once` test verifies one-row-one-tick invariant. `password_hash`/`token_hash` scrubbed from bundle via `SensitivePropertyNames`. `expires_at` computed at builder write time (`ReadyAt + 24h`); signed URL TTL recomputed as `expires_at - now` at GET time. FK cascade handles user deletion. Builder stores only exception type in `failure_reason` (PII safety). Reflection-based predicate correctly handles `ActorId` for audit log and `UserId`/`Id` for all other InExport tables. |
| Test Quality | PASS | All 8 task behaviors covered. 14 endpoint tests (exceeds >=12 requirement). Builder host tests cover queued→ready, key shape, expires-at, no-op, failure, and concurrent ticks. Bundle assembler tests cover six-table presence, empty user, cross-user isolation, and ActorId filter. Schema tests cover migration round-trip, status transitions, and index metadata. InMemoryObjectStore tests cover put/get, overwrite, missing-key, and signed-URL format. GCS tests skip correctly without credentials. S3 MinIO tests skip without `MINIO_ENDPOINT`. Footer cross-link test now genuinely verifies the Svelte template source. Playwright e2e covers happy-path, 429 rate-limit, and footer link. |

## Test Coverage

- CI gate (`./scripts/ci-local.sh --go`): PASS
- Backend `UserDataExport` filter: 14 endpoint tests pass
- `UserExportBuilderHostTests`: 6 tests pass (concurrent-ticks included)
- `UserExportBundleAssemblerTests`: 5 tests pass (ActorId filter, cross-user isolation, metadata)
- `UserDataExportSchemaTests`: 3 tests pass (migration round-trip, status transitions, index check)
- `InMemoryObjectStoreTests`: 4 tests pass
- `S3ObjectStoreTests`: 2 unit tests pass; 3 MinIO integration tests skip without `MINIO_ENDPOINT`
- `GcsObjectStoreTests`: 1 option-binding test passes; 2 signing tests skip without `GCS_SERVICE_ACCOUNT_KEY_PATH`
- `GdprAttributeScannerTests`: all existing tests pass + 2 new `UserIdColumnName` theory/fact pass
- `InternalRunExportBuilderEndpointTests`: 2 tests pass
- Web: 7 Vitest unit tests pass (footer test now non-tautological); Playwright e2e 3-test smoke spec present
- CHANGELOG.md: entry references M18-004 and v4-4 — PASS
- `docs/security/data-inventory.md`: cross-link to `/account/data` present — PASS

## Summary

All findings from prior iterations have been resolved. The previous Low-severity tautological
footer unit test was fixed by importing the Svelte template source via `?raw` and asserting the
href literal appears in it, which genuinely fails if the link is removed. The full implementation —
endpoint pair, background builder, object-store abstraction, reflection-driven bundle assembler,
concurrency token, pipe-based streaming, and SvelteKit client — meets all task behaviors and
Definition of Done items. Code is production-quality: sensitive columns scrubbed, PII-safe
failure reasons, best-effort signed-URL regeneration, and correct FK-cascade handling for user
deletion.
