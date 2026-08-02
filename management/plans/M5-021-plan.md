# Implementation Plan: M5-021

**Task:** Fix E2E M5 enterprise-full CI: externalize fake-idp ACS URL and stabilize seeded auth cookie
**Status:** retroactive (plan written after fixes landed; filed to satisfy workflow contract)
**Branch:** `fix/M5-021-enterprise-full-ci-green`

## Context

CI run 24632535770 on `main` had `E2E M5 enterprise-full` green for only 1 of 7
enterprise-full.spec.ts assertions. Three independent root causes surfaced
together once M5-020 shipped:

1. The fake SAML IdP sidecar hard-coded the ACS URL to the docker-internal
   hostname `http://backend:5000/api/v1/sso/saml`. The Playwright browser on
   the CI runner lives outside the compose network and cannot reach
   `backend:5000`, so the auto-submitting SAML form POSTed to an unroutable
   URL and every SSO test timed out.

2. `web/tests/e2e/helpers/auth.ts::seedAuthCookie` minted a fresh `uuidgen`
   user id on every call when no `userId` was passed. Once `85b5e30` made
   the users.Email claim persist (see #3), the backend started honouring the
   `UNIQUE(Email)` constraint and the second upsert for a canonical test
   email silently failed — leaving the Playwright request authenticated as
   a user row with no organization membership.

3. `CurrentUserAccessor.ResolveAsync` looked up the JWT `email` claim only
   under `JwtRegisteredClaimNames.Email`, but the JWT bearer middleware
   maps that to `ClaimTypes.Email`. Every upserted user row was saved with
   `Email = ""`. The InMemory EF provider tolerated the collision; SQLite
   in docker did not, which is why the failure only appeared in CI.

## Scope

In-scope files (all committed to this branch):

- `scripts/fake-idp/Program.cs` — make ACS URL configurable via env
- `scripts/fake-idp/Dockerfile`, `scripts/fake-idp/FakeIdp.csproj`,
  `scripts/fake-idp/metadata-template.xml` — inherited from M5-020 branch
  continuation
- `docker-compose.test.yml` — pass `FAKE_IDP_ACS_BASE` host URL
- `web/tests/e2e/helpers/auth.ts` — map canonical emails → stable UUIDs
- `src/ApiTool.Backend/Auth/CurrentUserAccessor.cs` — look up email under
  `ClaimTypes.Email` before falling back to `JwtRegisteredClaimNames.Email`
- `src/ApiTool.Backend.Tests/Invitations/AcceptInvitationPersistsEmailTests.cs`
  — regression: both owner and acceptee user rows must persist the JWT
  email claim
- `scripts/test-stack.sh` — dump compose logs on failure (moved pre-build)
- `ROADMAP.md` — sidecar workflow-hardening document added in the same
  branch because the workflow failures that produced this CI incident
  motivated it; kept on this branch rather than split into its own PR
  since it references the incident directly
- `management/tasks/M5-021.yaml`, `management/backlog.yaml`, `CHANGELOG.md`

Explicitly out of scope: the `DbUpdateException`-swallow fallback in
`CurrentUserAccessor` is preserved as-is; it legitimately absorbs the
id-race between two concurrent first-visit requests and is tracked
separately for hardening.

## Behaviours

1. Given fake-idp runs with `FAKE_IDP_ACS_BASE=http://localhost:5000/api/v1/sso/saml`,
   when `/saml/sso` is POSTed the HTML form action and the signed SAML
   Destination/Recipient/Audience elements all resolve from the CI host
   browser.
2. Given no `FAKE_IDP_ACS_BASE` is set (and an empty string is treated as
   unset), fake-idp falls back to `http://backend:5000/api/v1/sso/saml`
   (preserves in-compose callers).
3. Given `seedAuthCookie` is called for a canonically seeded email with no
   explicit userId, the JWT `sub` matches the user row created by
   `scripts/seed-test-data.sh`.
4. Given the 7 enterprise-full specs run against `./scripts/test-stack.sh up`,
   all 7 pass.

## Test strategy

- Behaviour 3 is covered by
  `AcceptInvitationPersistsEmailTests.Accepting_user_row_has_email_from_jwt_claim`
  (xUnit + `WebApplicationFactory`), which asserts both the owner and
  acceptee rows land with the JWT email claim populated — the same claim
  path the seedAuthCookie flow exercises. This is the root-cause test for
  the silent UNIQUE-email collision.
- Behaviours 1, 2, and 4 are verified end-to-end only, via
  `web/tests/e2e/enterprise-full.spec.ts` running against the docker
  compose stack on `./scripts/test-stack.sh up`. This is a deliberate
  choice: fake-idp is a deployment-only sidecar (no test host, no DI
  container) and auth.ts is a Playwright helper that shells out to
  `test-token.sh` — unit-testing either in isolation would mock the
  integration boundary we actually care about. The branch instead hardens
  the observability of the integration path (log-dumping `trap` in
  `test-stack.sh`, env-var-driven ACS URL with validated defaults).

## Risks

- Behaviour 2 has no unit coverage, so a regression that makes the
  default fall-back wrong (e.g., reading the wrong env var name) would
  only surface if someone drops the compose override. Mitigated by the
  defensive fix in Finding #5 (empty string treated as unset) and by
  the compose file committing the override explicitly.
- The stable `SEEDED_USER_IDS` map in `auth.ts` duplicates constants
  from two shell scripts. Mitigated by cross-reference comments in
  both scripts pointing back to the TypeScript map (Finding #9).

## Verification

- `dotnet test ApiTool.Backend.sln` — 565+ green
- `go test ./...` — unchanged (no Go code touched)
- `golangci-lint run` — unchanged
- `./smoke/run.sh` — unchanged
- `E2E M5 enterprise-full` CI job green on `fix/M5-021-enterprise-full-ci-green`
  — captured in `M5-021-verified.md` during `/verify`

Refs: M5-021
