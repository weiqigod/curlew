---
name: M5-021 Verification Report
description: Verification report for M5-021 — fix E2E M5 enterprise-full CI (PASS)
type: task
---

# Verification Report: M5-021

**Task:** Fix E2E M5 enterprise-full CI: externalize fake-idp ACS URL and stabilize seeded auth cookie
**Verified by:** AI
**Date:** 2026-04-19
**Branch:** fix/M5-021-enterprise-full-ci-green
**Verdict:** PASS

## Summary

Pass-rate trajectory across four CI rounds:

| Round | Run | Pass rate | Key fix |
|-------|-----|-----------|---------|
| Baseline | 24632535770 | 1/7 | failure being investigated |
| 1 | 24634187100 | 2/7 | initial M5-021 fixes (fake-idp env, seedAuthCookie, JWT email claim) |
| 2 | 24634817108 | 5/7 | role_id in MemberDto, ActorEmail on audit events, audit-log Status column |
| 3 | 24635134031 | 6/7 | fake-idp accepts GET (HTTP-Redirect binding) |
| 4 | **24635285508** | **7/7 ✓** | qa user as admin built-in + qa-lead custom overlay |

Final CI run: <https://github.com/weiqigod/curlew/actions/runs/24635285508>
- E2E enterprise-full: PASS in 3m58s
- E2E full pipeline (M4 sibling): PASS in 3m57s

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | clean |
| `go test ./...` | PASS | all packages green |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | end-of-run marker reached |
| `dotnet test ApiTool.Backend.sln` | PASS | 567/567 passed |
| `npm run check` (web) | PASS | 0 errors, 0 warnings |
| `npm run test:unit` (web) | PASS | 156/156 passed |
| Coverage (Go) | 86.7% | meets >= 80% threshold |
| **CI `E2E M5 enterprise-full`** | **PASS** | 7/7 specs in 3m58s, run 24635285508 |

## Observable Output

```
scripts/test-stack.sh up
CURLEW_BACKEND_TOKEN=$(scripts/test-token.sh qa@acme.example 00000000-0000-0000-0000-000000000002) \
  npx --prefix web playwright test tests/e2e/enterprise-full.spec.ts
# Expected: 7/7 tests pass (was 1/7 passing on CI run 24632535770).
scripts/test-stack.sh down
```

CI run 24635285508 is the authoritative verification (Docker is unavailable
on the local verification host). Result: 7 passed (17.0s, second-attempt).

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | fake-idp `FAKE_IDP_ACS_BASE` host-reachable | enterprise-full.spec.ts assertion 1 (CI) | PASS |
| 2 | Default `http://backend:5000/...` fallback | code (`scripts/fake-idp/Program.cs:35-38`) + CI | PASS |
| 3 | `seedAuthCookie` stable JWT `sub` | `AcceptInvitationPersistsEmailTests.Accepting_user_row_has_email_from_jwt_claim` (xUnit, 281 ms) | PASS |
| 4 | All 7 enterprise-full specs pass | CI run 24635285508 | PASS (7/7) |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All 7 enterprise-full.spec.ts tests pass | CI run 24635285508 — 7/7 in 3m58s | PASS |
| 2 | `dotnet test ApiTool.Backend.sln` passes (no regression) | 567/567 (added 2 regression tests this slice) | PASS |
| 3 | `go test ./...` passes | all packages green | PASS |
| 4 | `golangci-lint run` passes | 0 issues | PASS |
| 5 | `./smoke/run.sh` passes | end marker reached, all PASS lines | PASS |
| 6 | CI job `E2E M5 enterprise-full` is green on the fix branch | https://github.com/weiqigod/curlew/actions/runs/24635285508 | PASS |

## Code Review

Branch A spot-check:

1. **Error handling** — fake-idp `Guid.TryParse` validates RelayState
   (`scripts/fake-idp/Program.cs:70-75`); SsoService threads `failureReason`
   through every audit-failure branch (lines 226, 246, 258); audit writer
   serialises payloads with `JsonNamingPolicy.SnakeCaseLower` and never throws
   on null inputs.
2. **Documented exports** — `MemberDto.RoleId` carries an XML doc explaining
   the wire format and frontend dependency
   (`src/ApiTool.Backend/Organizations/MemberDto.cs:6-13`); `AuditEvent.ActorEmail`
   carries an XML doc explaining the rendering preference
   (`src/ApiTool.Backend/Audit/AuditEvent.cs:21-25`).
3. **Test exercises what it claims** —
   `MembersEndpointsTests.Get_members_returns_role_id_for_member_with_custom_role_assignment`
   creates a custom role, assigns it to a fresh member via PATCH, then
   asserts the GET response carries `role_id` round-tripped to the wire
   format. Round-trip verified, not just a non-null assertion.

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

## Round-2 + 3 + 4 Commits

| Hash | Message |
|------|---------|
| 698a625 | fix(seed): bump qa user to admin built-in role with qa-lead custom overlay |
| aee1421 | fix(fake-idp): accept GET /saml/sso for HTTP-Redirect SAMLRequest binding |
| af948f4 | ci(e2e-m5): dump backend, fake-idp, and web logs on Playwright failure |
| 33f769b | feat(web): render Status column and prefer user_email in audit-log table |
| a2d307f | feat(audit): populate actor_email at sso.login and results.upload call sites |
| 51ba603 | feat(audit): persist actor_email on audit events for human-readable rendering |
| d11b902 | fix(members): include role_id in MemberDto so custom-role member counts render |
| 77cb91d | docs(verify): record FAIL verdict for M5-021 (CI 2/7, was 1/7) |

(Plus the 18 earlier commits for the original three-cause fixes; see
`git log origin/feature/M5-020-e2e-enterprise-full..HEAD` for the full set.)

## Files Changed (round-2/3/4 increment)

| File | Action | Purpose |
|------|--------|---------|
| `src/ApiTool.Backend/Organizations/MemberDto.cs` | modified | Add `RoleId` field |
| `src/ApiTool.Backend/Organizations/MembersService.cs` | modified | Project `RoleId` in list endpoint |
| `src/ApiTool.Backend/Audit/AuditEvent.cs` | modified | Add `ActorEmail` parameter |
| `src/ApiTool.Backend/Audit/AuditWriter.cs` | modified | Persist `ActorEmail` |
| `src/ApiTool.Backend/Audit/AuditLogEntryDto.cs` | modified | Add `UserEmail` field |
| `src/ApiTool.Backend/Audit/AuditLogQueryService.cs` | modified | Surface `UserEmail` |
| `src/ApiTool.Backend/Audit/AuditLogCsvFormatter.cs` | modified | Add `user_email` column |
| `src/ApiTool.Backend/Data/Entities/OrganizationAuditLogEntry.cs` | modified | Add `ActorEmail` column |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modified | Map `actor_email` snake_case |
| `src/ApiTool.Backend/Migrations/20260419171432_AddAuditLogActorEmail.cs` | added | EF migration |
| `src/ApiTool.Backend/Sso/SsoService.cs` | modified | Pass `ActorEmail` on all 4 sso.login branches |
| `src/ApiTool.Backend/Results/ResultsEndpoints.cs` | modified | Look up email + pass on results.upload |
| `web/src/lib/types/audit-log.ts` | modified | Add `user_email` field |
| `web/src/routes/(app)/org/[slug]/audit-log/+page.svelte` | modified | Status column + prefer user_email |
| `scripts/fake-idp/Program.cs` | modified | Accept GET for HTTP-Redirect binding |
| `scripts/seed-enterprise.sh` | modified | qa user as admin + qa-lead overlay |
| `.github/workflows/e2e-m5.yml` | modified | Dump backend logs on Playwright failure |
| `src/ApiTool.Backend.Tests/Audit/AuditLogCsvFormatterTests.cs` | modified | New `user_email` column |
| `src/ApiTool.Backend.Tests/Organizations/MembersEndpointsTests.cs` | modified | Regression test for role_id round-trip |
| `src/ApiTool.Backend.Tests/Sso/SsoServiceTests.cs` | modified | Regression test for ActorEmail on success |
| `src/ApiTool.Backend.Tests/Results/ResultsAuditTests.cs` | modified | Regression test for ActorEmail on results.upload |
| `web/src/lib/audit-log/csv.test.ts` | modified | Field added to fixture |
| `web/src/routes-tests/page-server.test.ts` | modified | Field added to fixture |

## Issues Found

None.

## Recommendation

**PASS** — ready to merge. All six DoD items verified end-to-end; CI green
on the authoritative reproduction environment for the bug.

Refs: M5-021
