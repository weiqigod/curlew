# Code Review: M18-012 (iteration 5)

**Task:** M18 end-to-end convergence: telemetry → export → deletion → anonymisation → audit-log export → encrypted columns
**Reviewer:** AI
**Date:** 2026-05-19
**Branch:** feature/M18-012-e2e-convergence

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All three C# endpoints return structured 400/404 error responses with `CancellationToken` propagation. Shell orchestrator uses `fail()` helper for every hard assertion in steps 1–12. No swallowed errors. |
| Input Validation | PASS | `install_id` validates UUID format before DB query (400 on empty or malformed). `user_id` guards null and `Guid.Empty` (400). `DaysAgo` defaults to 31 safely. TypeScript helpers throw on non-ok HTTP responses with status + body text included. |
| Naming | PASS | No stuttering. Doc comments on all exported types and public methods. `MapInternal*` extension method pattern followed consistently across all three endpoints. Interfaces use single-word lowercase package names throughout. |
| Code Organization | PASS | All three endpoints follow the established `MapInternal*` internal pattern; `Program.cs` wiring is additive (lines 937–939); `InternalAccessFilter` from `Licensing.Keys` is applied correctly; internal package boundaries respected; no circular dependencies introduced. |
| Correctness | PASS | The iteration-4 critical finding (missing `mint-reauth-token` hook) is fully resolved. `InternalMintReauthTokenEndpoint.cs` correctly uses `AuthTokenIssuer.Mint(AuthTokenIssuer.DeletionReauthPrefix)` to produce a real `drto_`-prefixed token, persists the hash to `DeletionReauthTokens`, and returns plaintext + `expires_at`. `m18-e2e.sh` uses the hook in steps 6 and 6-re-initiate. The full 12-step + Playwright scenario can now execute. |
| Test Quality | PASS | 7 xUnit facts for `InternalListTelemetryEventsEndpointTests` (known-id rows, zero-count, missing param, malformed param, Testing-env registration, Production 404, response shape). 8 facts for `InternalBackdateDeletionEndpointTests` (backdate mutation, 404-no-pending, 400-missing-id, 400-empty-id, default-31, Testing-env registration, Production 404, response shape). 7 facts for `InternalMintReauthTokenEndpointTests` (200+drto token, hash persisted, 404-unknown-user, 400-missing-id, 400-empty-id, Testing-env registration, Production 404). 5 Playwright assertions with correct env-var skip guards and correct `M18_ENTERPRISE_TOKEN` routing for the Enterprise audit-log stream. All `Returns_404_in_Production` cases present. |

## Pre-Audit Gate

`./scripts/ci-local.sh --go` passed. All Go packages build, test, race-detect, and lint clean. Go test coverage: **87.1%** (above the 80% gate).

## Test Coverage

- Go coverage: 87.1% overall (above 80% gate)
- C# new test coverage: 7 facts for `InternalListTelemetryEventsEndpointTests`; 8 facts for `InternalBackdateDeletionEndpointTests`; 7 facts for `InternalMintReauthTokenEndpointTests` — all paths covered including Production-isolation and empty-UUID guards
- Playwright: 5 assertions in `m18-compliance.spec.ts`, all with appropriate env-var skip guards

## Spec Behavior Coverage

| Behavior | Covered By | Status |
|----------|-----------|--------|
| 12-step convergence exits 0 | `m18-e2e.sh` end-to-end (steps 1–12 + Playwright step 13) | COVERED — all blocking gaps resolved |
| telemetry_events ≥1 row with `run.completed` for install_id | `m18-e2e.sh` step 4 + Playwright assertion 3 | COVERED |
| Export bundle carries InExport tables | `m18-e2e.sh` step 5 + Playwright assertion 2 | COVERED |
| Audit-log rows anonymised (`actor_email` matches `deleted-user-[0-9a-f]{8}`) | `m18-e2e.sh` step 8 | COVERED (deletion chain now executes via `mint-reauth-token` hook) |
| `user.anonymised` audit-log row present | `m18-e2e.sh` step 9 | COVERED |
| Enterprise JSONL streaming export is `chunked + application/x-ndjson` | `m18-e2e.sh` step 10 + Playwright assertion 5 | COVERED |
| `team_vaults` / `schedules.env_vars` round-trip proof | `m18-e2e.sh` step 11 | COVERED |
| `curlew telemetry delete-request` removes install_id + posts marker | `m18-e2e.sh` step 12 | COVERED |
| Happy-path only; individual failure modes not asserted | Test structure / skip guards | COVERED |

## Summary

The sole finding from iteration 4 — the missing `POST /api/v1/internal/test-hooks/mint-reauth-token` backend hook — has been fully implemented. The new endpoint correctly mints `drto_`-prefixed reauth tokens for passwordless seed-refresh test users, is gated by `InternalAccessFilter` and never registered in Production, and is covered by 7 xUnit facts. All three new internal endpoints are wired in `Program.cs`, tested exhaustively, and follow the established `MapInternal*` pattern. The shell orchestrator and Playwright spec are complete, coherent, and aligned with the task observable. No new findings identified.
