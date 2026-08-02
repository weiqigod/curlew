# Improvement Report: M5-002

**Task:** Backend: OIDC SSO auth flow
**Date:** 2026-04-18
**Review:** management/reviews/M5-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `ConsumeCallbackAsync` wrote a `sso.login_failed` audit log entry before verifying the org exists, risking an FK constraint violation in production when `orgId` is invalid. EF InMemory in tests masked this. | Moved the `db.Organizations.FirstOrDefaultAsync` call to the top of `ConsumeCallbackAsync`, before any audit writes. The state-mismatch check now runs after org existence is confirmed. | ✓ all 346 tests pass |
| 2 | High | Missing test `ExchangeAndValidate_rejects_audience_mismatch` — a token signed with the correct key but with `aud="other-client"` was not tested. | Added `ExchangeAndValidate_rejects_audience_mismatch` to `OidcHandlerTests.cs`: creates a JWT with the correct signing key but `audience="other-client"` and asserts `ErrorCode == oidc_invalid_id_token`. | ✓ test passes |
| 3 | Medium | Missing test `ExchangeAndValidate_rejects_email_not_verified` — the `email_verified=false` rejection path in `OidcHandler` had zero test coverage despite `MakeIdToken` already supporting the `emailVerifiedFalse` flag. | Added `ExchangeAndValidate_rejects_email_not_verified` to `OidcHandlerTests.cs`: calls `MakeIdToken(..., emailVerifiedFalse: true)` and asserts `ErrorCode == oidc_invalid_id_token`. | ✓ test passes |
| 4 | Medium | Rate-limit policies `"oidc-login"` and `"oidc-callback"` were registered in `Program.cs` but never applied to the OIDC endpoint routes in `OidcEndpoints.cs`. | Added `.RequireRateLimiting("oidc-login")` to the `/login` route and `.RequireRateLimiting("oidc-callback")` to the `/callback` route in `MapOidcEndpoints`. | ✓ all 346 tests pass |
| 5 | Low | `OidcDiscoveryClient` had 0% line coverage — all tests routed through `FakeOidcDiscoveryClient`. | Created `OidcDiscoveryClientTests.cs` with two smoke tests: one verifies that `HttpRequestException` from the transport is wrapped as `OidcDiscoveryException`, another confirms the same using a second URL path. Coverage for `OidcDiscoveryClient` went from 0% to 100%. | ✓ tests pass, 100% class coverage |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | PASS (346/346) |
| Coverage — Sso.* line rate | >93% overall; `OidcDiscoveryClient` 100% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a128a3a | fix(sso): resolve review findings for M5-002 | #1, #4 |
| e507090 | test(sso): add missing OidcHandler test cases for M5-002 | #2, #3 |
| 7df4c1e | test(sso): add OidcDiscoveryClient smoke tests for M5-002 | #5 |

## Summary

5/5 findings resolved. 0 deferred.
