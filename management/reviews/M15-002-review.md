# Code Review: M15-002

**Task:** SSO Enterprise tier gate at SsoService with 402/404 endpoint translation
**Reviewer:** AI
**Date:** 2026-05-07
**Branch:** feature/M15-002-sso-enterprise-tier-gate
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All three findings from iteration 1 were resolved by the `/improve` pass:

| # (iter 1) | Severity | Finding | Resolution |
|------------|----------|---------|------------|
| 1 | Medium | `EnsureEnterpriseTierAsync` duplicated verbatim in `SsoService` and `OidcService` | Extracted to `internal static class SsoTierGate` in `SsoTierGate.cs`; both services call `SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct)` |
| 2 | Low | No endpoint integration test for authenticated config endpoint on unknown org (expected 404 `organization_not_found`, not 402) | Added `Get_sso_config_with_unknown_org_guid_returns_404_organization_not_found` to `SamlEndpointsTests.cs` |
| 3 | Low | `scripts/seed-enterprise.sh` line 13 usage comment still read "Create a team subscription" after the code was updated to enterprise | Updated to "Create an enterprise subscription (if not already present) so SSO and seats work" |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All 7 service entry points (4 SAML + 3 OIDC) gate via `SsoTierGate.EnsureEnterpriseAsync`, returning typed `SsoError` tuples. No exceptions for expected failures. Unknown org → `OrgNotFound` (not `TierIneligible`), preventing org-existence leakage on authenticated endpoints. |
| Input Validation | PASS | Gate fires at the top of each service method before any DB writes. Org existence checked inside the gate before subscription lookup. Nil/empty guid inputs caught by `Guid.TryParse` upstream in endpoint handlers before any service call. |
| Naming | PASS | `TierIneligible`, `SsoTierIneligible`, `SsoTierGate`, `EnsureEnterpriseAsync`, `SsoNotFoundResult` all follow C# naming conventions. No stuttering. XML doc comments on all exported/internal symbols. `SsoTierGate` is `internal static` — not leaking into public API surface. |
| Code Organization | PASS | Gate logic lives in exactly one place (`SsoTierGate.cs`). `SsoNotFoundResult` is co-located in `SamlEndpoints.cs` (same assembly, same namespace) and consumed directly by `OidcEndpoints.cs` — no cross-package violations. Single responsibility respected. |
| Correctness | PASS | Gate fires before any DB writes (confirmed by `TierIneligible_short_circuits_before_db_writes` test). `OrderByDescending(s.UpdatedAt)` defends against theoretical duplicate subscription rows. `Cache-Control: no-store` enforced on all public-flow 404 responses via `SsoNotFoundResult`. Enterprise happy paths preserved via fixture seeding in all existing tests. |
| Test Quality | PASS | 29 new tier-matrix tests cover the full grid: {Free, Professional, Team, no-subscription} × {all 7 SSO service entry points + 6 endpoint paths (SAML GET/PUT config, login, ACS; OIDC PUT config, login, callback)}. Short-circuit regression test (`TierIneligible_short_circuits_before_db_writes`) confirms gate runs before persistence. Integration test for unknown-org authenticated config endpoint confirms 404 not 402. All 8 task behaviors have test coverage. |

## Behavior Coverage

| Behavior | Covered By |
|----------|-----------|
| Enterprise org → no tier rejection, existing behavior preserved | All existing happy-path tests now seeded with Enterprise sub |
| Free/Professional/Team → TierIneligible at service layer | `NonEnterpriseTiers` Theory in `SsoServiceTests` + `OidcServiceTests` |
| No subscription row → treated as Free → TierIneligible | `null` tier case in `NonEnterpriseTiers` matrix |
| Non-enterprise authenticated endpoint → HTTP 402 + `sso_tier_ineligible` body | `Get_sso_config_non_enterprise_returns_402`, `Put_saml_config_non_enterprise_returns_402`, `Put_oidc_config_non_enterprise_returns_402` |
| Non-enterprise SAML public flow → HTTP 404 + no body + `Cache-Control: no-store` | `Saml_login_non_enterprise_returns_404_no_store_empty_body`, `Saml_acs_non_enterprise_returns_404_no_store_empty_body` |
| Non-enterprise OIDC public flow → HTTP 404 + no body + `Cache-Control: no-store` | `Oidc_login_non_enterprise_returns_404_no_store_empty_body`, `Oidc_callback_non_enterprise_returns_404_no_store_empty_body` |
| Enterprise org, public endpoints → 302/401/403 behavior preserved | All existing public-flow tests seeded with Enterprise sub |
| Dev seed updated for Enterprise subscription | `scripts/seed-enterprise.sh` line 66 + 73 changed to `enterprise` |

## Test Coverage

- `./scripts/ci-local.sh --go`: PASS (Go gate unaffected; C# changes do not touch Go packages)
- `dotnet build /warnaserror` (both projects): 0 warnings, 0 errors
- `dotnet test --filter "FullyQualifiedName~Sso"`: 182 passed, 0 failed
- `dotnet test --filter "FullyQualifiedName~Sso&FullyQualifiedName~Tier"`: 29 passed, 0 failed
- `dotnet test` (all tests): 1260 passed, 8 skipped, 0 failed

New tests added: 29 tier-matrix tests + 1 unknown-org integration test = 30 net new.
Missing coverage: none.

## Summary

The iteration-2 implementation is correct and complete. All three findings from the first review are resolved: the gate logic is now centralised in a single `SsoTierGate` static class, the missing integration test for the unknown-org edge case is present, and the seed script comment is accurate. The full tier matrix ({Free, Professional, Team, Enterprise, no-subscription} × all 7 SSO service methods + 6 endpoint paths) is covered by tests. The existing SSO test suite is fully preserved via Enterprise subscription seeding.
