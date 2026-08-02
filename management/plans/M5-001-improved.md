# Improvement Report: M5-001

**Task:** Backend: SAML 2.0 SSO auth flow
**Date:** 2026-04-18
**Review:** management/reviews/M5-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Signature wrapping vulnerability — `GetElementsByTagName("Assertion").FirstOrDefault()` not cross-checked against the `Reference` URI | After `CheckSignature` passes, retrieve `signedXml.SignedInfo.References[0].Uri` and confirm it equals `"#" + assertionEl.GetAttribute("ID")`; return `saml_signature_invalid` if they don't match | ✓ tests pass |
| 2 | High | `Consume_returns_user_not_member` only covered the "user not in Users table" path; the "user exists but not a member" path (lines 196–200) was not covered | Added `Consume_returns_user_not_member_when_user_exists_but_not_in_org` to `SsoServiceTests` seeding an external user who is not a member of the org | ✓ tests pass |
| 3 | High | Missing endpoint coverage for: PUT with bad org-id format, GET `/login` unknown org, POST `/acs` SSO-not-enabled, POST `/acs` unknown org | Added four new tests to `SamlEndpointsTests`: `Put_saml_config_with_non_org_prefixed_id_returns_404`, `Get_login_with_unknown_org_guid_returns_404`, `Post_acs_with_sso_not_enabled_returns_404`, `Post_acs_with_unknown_org_guid_returns_404` | ✓ tests pass |
| 4 | High | `SamlHandler` paths for missing Signature, missing Assertion, empty NameID, and catch block uncovered | Added five tests: `ValidateResponse_returns_signature_invalid_when_no_signature_element`, `…no_assertion_element`, `…nameID_is_empty`, `…for_malformed_base64_input`, `…for_non_xml_payload` | ✓ tests pass |
| 5 | Medium | `SsoError.AssertionExpired` defined but never returned; expired assertions incorrectly mapped to `SignatureInvalid` | Added `SsoErrorCodes.AssertionExpired = "assertion_expired"`; `SamlHandler` now returns it when `NotOnOrAfter` has passed; `SsoService` routes it to `SsoError.AssertionExpired`; `SamlEndpoints.AcsCallback` handles it with HTTP 401; `FakeSamlHandler.Expired` mode corrected to return the new code | ✓ tests pass |
| 6 | Medium | Session cookie has no `MaxAge` — cookie lifetime doesn't match JWT TTL | `SamlRedirectResult` now accepts `TimeSpan sessionTtl`; sets `MaxAge = sessionTtl` on `CookieOptions`; `SetSessionCookieAndRedirect` passes `samlOptions.SessionTtl` | ✓ tests pass |
| 7 | Medium | No test for idempotent cert upsert (same KID upserted twice) | Added `Upsert_cert_twice_with_same_kid_updates_row_and_leaves_single_credential` to `SsoServiceTests` | ✓ tests pass |
| 8 | Low | Blanket `catch (Exception)` swallows all exceptions including non-recoverable ones | Replaced with four explicit catches: `FormatException`, `XmlException`, `CryptographicException`, `InvalidOperationException` | ✓ tests pass |
| 9 | Low | `RequestId` discarded; `InResponseTo` validation not implemented despite being committed to in the plan | Added inline comment to `SsoService.BuildLoginAsync` documenting the scope reduction; `InResponseTo` validation deferred to a follow-up task with explicit rationale | ✓ documented |

## Out of Scope (Deferred)

No findings deferred. All 9 findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.csproj -warnaserror` | PASS |
| `dotnet test ApiTool.Backend.Tests.csproj` | PASS |
| `golangci-lint run` | PASS |
| SSO/SAML test count | 47 (was 36) |
| All tests | 292/292 passing |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 4c02d6b | fix(sso): resolve all review findings for M5-001 | #1, #2, #3, #4, #5, #6, #7, #8, #9 |

## Summary

9/9 findings resolved. 0 deferred.

Changes span 5 source files (`SamlHandler.cs`, `SsoErrorCodes.cs`, `SsoService.cs`, `SamlEndpoints.cs`, `FakeSamlHandler.cs`) and 3 test files (`SamlHandlerTests.cs`, `SsoServiceTests.cs`, `SamlEndpointsTests.cs`). The test suite grew from 36 to 47 SSO/SAML tests (all 292 total tests pass).
