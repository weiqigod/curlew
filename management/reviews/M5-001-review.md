# Code Review: M5-001

**Task:** Backend: SAML 2.0 SSO auth flow
**Reviewer:** AI
**Date:** 2026-04-18
**Branch:** feature/M5-001-backend-saml-sso
**Iteration:** 2 (re-review after improvements)

## Verdict: PASS

## Findings

No findings. All nine issues from review iteration 1 have been resolved:

| # | Prior Finding | Resolution |
|---|---------------|-----------|
| 1 | Critical — Signature wrapping vulnerability in `SamlHandler` | Fixed: `refUri` is now cross-checked against `"#" + assertionId` before using the assertion (lines 113–115 of `SamlHandler.cs`). |
| 2 | High — Missing test for "user exists but not member" code path | Fixed: `Consume_returns_user_not_member_when_user_exists_but_not_in_org` added to `SsoServiceTests.cs`. |
| 3 | High — Missing endpoint-level error branch tests | Fixed: Four new endpoint tests added (`Put_saml_config_with_non_org_prefixed_id_returns_404`, `Get_login_with_unknown_org_guid_returns_404`, `Post_acs_with_sso_not_enabled_returns_404`, `Post_acs_with_unknown_org_guid_returns_404`). |
| 4 | High — `SamlHandler` failure paths uncovered | Fixed: Nine new handler tests cover no-signature, no-assertion, empty NameID, malformed base64, and non-XML inputs. |
| 5 | Medium — `SsoError.AssertionExpired` dead enum value | Fixed: `SamlHandler` now returns `SsoErrorCodes.AssertionExpired` on expired assertions; `SsoService` maps it to `SsoError.AssertionExpired`; endpoint returns 401 with distinct code. |
| 6 | Medium — Session cookie missing `MaxAge` | Fixed: `SamlRedirectResult` now sets `MaxAge = sessionTtl` on `CookieOptions`. |
| 7 | Medium — No idempotent cert upsert test | Fixed: `Upsert_cert_twice_with_same_kid_updates_row_and_leaves_single_credential` added to `SsoServiceTests.cs`. |
| 8 | Low — Broad `catch (Exception)` in `SamlHandler` | Fixed: Catch narrowed to `FormatException`, `XmlException`, `CryptographicException`, and `InvalidOperationException`. |
| 9 | Low — `RequestId` discarded silently | Addressed: An explicit comment in `SsoService.BuildLoginAsync` documents the intentional scope reduction (InResponseTo validation deferred to a follow-up task). |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Structured `SsoError` enum + typed result tuples throughout; no swallowed errors; specific exception types caught in `SamlHandler`. |
| Input Validation | PASS | Required fields validated in `SsoService.UpsertSamlConfigAsync`; org-id format checked in all endpoint handlers; null SAMLResponse coalesced to empty string. |
| Naming | PASS | No stuttering; doc comments on all exported types and members; snake_case error codes match spec; `-er` suffix on `ISamlHandler`. |
| Code Organization | PASS | All SSO code in `Sso/` namespace; `ISamlHandler` abstraction cleanly separates BCL crypto from business logic; `FakeSamlHandler` in test project only; no circular dependencies. |
| Correctness | PASS | Signature wrapping fixed; cookie `MaxAge` aligned with JWT TTL; `AssertionExpired` wired end-to-end; `InResponseTo` deferral documented. |
| Test Quality | PASS | All 8 behaviors from task YAML covered; error branches for all three endpoints tested; `SamlHandler` failure paths exercised; both `UserNotMember` code paths covered; cert idempotency verified. |

## Test Coverage

- SSO-filtered test run: **Passed: 47, Failed: 0** (`--filter FullyQualifiedName~Sso`)
- Observable filter (`FullyQualifiedName~Sso&FullyQualifiedName~Saml`): **Passed: 25, Failed: 0** (requirement >= 10)
- Build: clean, **0 warnings, 0 errors** (`-warnaserror`)
- All 8 task behaviors explicitly covered by named tests.

## Summary

All nine findings from the first review iteration have been resolved. The signature-wrapping vulnerability (the critical finding) is correctly patched: the code now cross-checks the signature's `Reference` URI against the assertion element's `ID` before trusting the assertion's content. Test coverage now reaches all meaningful branches in `SamlHandler`, `SsoService`, and the three SAML endpoints. The implementation is architecturally sound, builds without warnings, and all tests pass.
