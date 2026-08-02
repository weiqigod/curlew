# Improvement Report: M15-002

**Task:** SSO Enterprise tier gate at SsoService with 402/404 endpoint translation
**Date:** 2026-05-07
**Review:** management/reviews/M15-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `EnsureEnterpriseTierAsync` duplicated verbatim in `SsoService` and `OidcService` — any future gate change (e.g. status filter) must be applied in two places | Extracted to `internal static class SsoTierGate` in `src/ApiTool.Backend/Sso/SsoTierGate.cs`. Both services now call `SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct)`. Private methods removed from both service classes. | ✓ builds 0 warnings, 1260 tests pass |
| 2 | Low | No endpoint integration test for `GET /api/v1/organizations/{valid-but-nonexistent-guid}/sso → 404 organization_not_found` | Added `Get_sso_config_with_unknown_org_guid_returns_404_organization_not_found` to `SamlEndpointsTests.cs`. Test creates a well-formed but non-existent org GUID and asserts 404 + `code=organization_not_found`. | ✓ test passes |
| 3 | Low | `scripts/seed-enterprise.sh` line 13 usage comment still read "Create a team subscription" after the code was updated to enterprise | Updated line 13 to `# 3. Create an enterprise subscription (if not already present) so SSO and seats work` | ✓ comment matches code |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build /warnaserror` (backend) | PASS |
| `dotnet build /warnaserror` (tests) | PASS |
| `dotnet test --filter "FullyQualifiedName~Sso"` | PASS — 182 passed, 0 failed |
| `dotnet test` (all tests) | PASS — 1260 passed, 8 skipped, 0 failed |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS — 0 issues |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `33d8fe1a` | refactor(sso): extract EnsureEnterpriseTierAsync into SsoTierGate | #1 |
| `af414c49` | fix(sso): add unknown-org 404 integration test; fix seed comment | #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
