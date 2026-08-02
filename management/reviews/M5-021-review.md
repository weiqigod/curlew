# Code Review: M5-021 (iteration 3)

**Task:** Fix E2E M5 enterprise-full CI: externalize fake-idp ACS URL and stabilize seeded auth cookie
**Reviewer:** AI
**Date:** 2026-04-19
**Branch:** fix/M5-021-enterprise-full-ci-green

## Verdict: PASS ✓

All eleven findings across iterations 1 and 2 are resolved and
commit-traceable. The code changes are correct, narrowly scoped,
and well-tested given the deployment-only nature of fake-idp and
the Playwright helper.

## Findings

None.

## Previous-iteration Findings (All Resolved)

| Iter | # | Finding | Fix commit | Current state |
|------|---|---------|------------|---------------|
| 1 | 1 | Scope block missing 4 files; CurrentUserAccessor note wrong | `4de9515` | `management/tasks/M5-021.yaml:21-39` enumerates all 10 touched files; explicitly states the email-claim mapping IS fixed here |
| 1 | 2 | No `M5-021-plan.md` | `b9a7bfe` | Plan exists with three root causes, scope, behaviours, test strategy, risks |
| 1 | 3 | Unrelated ROADMAP.md | `4de9515` | Task YAML scope (lines 31-33) treats it as a design-debrief sidecar |
| 1 | 4 | Test class missing `Tests` suffix | `a5cd37b` | `AcceptInvitationPersistsEmailTests.cs:20` renamed |
| 1 | 5 | Empty `FAKE_IDP_ACS_BASE` accepted | `4842798` | `scripts/fake-idp/Program.cs:36-38` uses `IsNullOrWhiteSpace` + `TrimEnd('/')` |
| 1 | 6 | Behaviours 1/2/3 no unit coverage | `b9a7bfe` | Plan documents integration-only rationale; behaviour 3 covered by new xUnit regression |
| 1 | 7 | `trap ERR` installed after build | `a9894da` | `scripts/test-stack.sh:49` installs trap before build with comment |
| 1 | 8 | Single 1500-char CHANGELOG entry | `691ac32` | `CHANGELOG.md:10-12` split into three bullets |
| 1 | 9 | `SEEDED_USER_IDS` duplication undocumented | `3142d61` | Both `seed-test-data.sh:19-22` and `seed-enterprise.sh:34-37` carry `SYNC-NOTE` cross-refs |
| 1 | 10 | DoD has no citable artifact | `4de9515` | `management/tasks/M5-021.yaml:46` requires green CI run link in verify report |
| 2 | 1 | Task status stale as `planned` | `950e8ad` | `management/tasks/M5-021.yaml:3` and `management/backlog.yaml:797` both `review`; backlog entry also gained `started_date: 2026-04-19` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return/surface rather than panicking. `scripts/test-stack.sh` uses `set -euo pipefail` + ERR trap; shell exits are explicit. `CurrentUserAccessor` swallows `DbUpdateException` only on the documented race path and is explicitly tracked as a follow-up. |
| Input Validation | PASS | `FAKE_IDP_ACS_BASE` whitespace-guarded and trailing-slash-trimmed (`Program.cs:36-38`). `RelayState` is `Guid.TryParse`-validated (`Program.cs:70-75`). JWT claims defensively resolved under both `ClaimTypes.Email` and `JwtRegisteredClaimNames.Email` (`CurrentUserAccessor.cs:40-43`). |
| Naming | PASS | `AcceptInvitationPersistsEmailTests` matches the suite's `*Tests` convention. Exported identifiers (`CurrentUserAccessor`, `ResolveAsync`, `SEEDED_USER_IDS`) are clean and documented. |
| Code Organization | PASS | Changes confined to the right layers: scripts/fake-idp (infra), docker-compose.test.yml (compose override), web/tests/e2e/helpers/auth.ts (test helper), src/ApiTool.Backend/Auth/CurrentUserAccessor.cs (auth), src/ApiTool.Backend.Tests (regression test). No cross-package reach-in. |
| Correctness | PASS | The three fixes correctly target their respective root causes: (a) JWT claim-type mapping (`ClaimTypes.Email` vs `JwtRegisteredClaimNames.Email`), (b) host-vs-in-compose ACS URL via env indirection, (c) stable JWT `sub` via `SEEDED_USER_IDS` map. Canonical UUIDs verified to match between `auth.ts`, `seed-test-data.sh` (OWNER), and `seed-enterprise.sh` (QA). |
| Test Quality | PASS | `AcceptInvitationPersistsEmailTests.Accepting_user_row_has_email_from_jwt_claim` is a genuine integration test via `WebApplicationFactory` that exercises the full invitation-accept flow and asserts both owner and acceptee rows persist the JWT email — robust against InMemory-provider corner cases because it verifies the populated value directly. Behaviours 1/2/4 are deployment-integration surfaces where unit coverage would mock the boundary we care about (documented rationale in `M5-021-plan.md`). |

## Test Coverage

- `AcceptInvitationPersistsEmailTests.Accepting_user_row_has_email_from_jwt_claim` — exercises JWT-middleware claim-mapping path for both owner and acceptee, asserts neither row lands with `Email = ""`. Correct regression for behaviour 3.
- `scripts/fake-idp/Program.cs` env-driven ACS URL: covered only via `enterprise-full.spec.ts` (documented integration-only choice).
- `web/tests/e2e/helpers/auth.ts` `SEEDED_USER_IDS` map: covered only via `enterprise-full.spec.ts` (same rationale).
- Behaviour 4 (7/7 enterprise-full specs pass): verifiable only through the CI job `E2E M5 enterprise-full` on this branch; green-run URL to be captured in `M5-021-verified.md` during `/verify`.

## Summary

No findings. Code meets all standards. All ten iter-1 findings and
the one iter-2 workflow-hygiene finding are resolved and
commit-traceable. Standards compliance passes in all six categories.
The branch is ready for `/verify` to capture the green CI run URL
and transition the task to `done`.

→ Run `/verify M5-021` to complete the task.
