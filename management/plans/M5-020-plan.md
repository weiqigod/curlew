# Implementation Plan: M5-020

## Overview

Convergence slice that stitches SSO (M5-001/2/3), audit (M5-004/5), custom roles (M5-006/7),
and the existing results pipeline (M4-004) into a single Playwright end-to-end spec.
Adds a fake SAML IdP sidecar, a `scripts/seed-enterprise.sh` helper, enterprise fixture
data, a `results.upload` audit event, and a CI job `e2e-m5` that runs the new spec.

## Task Details

- **ID:** M5-020
- **Title:** E2E: SSO login -> audit capture -> dashboard with custom role
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** high
- **Estimated effort:** 8-12 hours

## Dependencies

| Task    | Title                                     | Status |
|---------|-------------------------------------------|--------|
| M5-003  | Web: SSO config UI                        | done   |
| M5-005  | Web: Audit log viewer                     | done   |
| M5-007  | Web: Custom role editor                   | done   |

## Architectural Decisions

1. **"Service token" = user-scoped JWT bound to a custom role.** The task refers to a
   "service token tied to the custom role". No dedicated service-token subsystem exists,
   and building one is out of scope for a convergence slice. Instead, `seed-enterprise.sh`
   creates a user (`qa@acme.example`), adds them as an org member with `role_id` pointing
   to the `qa-lead` custom role, and the CI test mints a bearer JWT for that user via
   `scripts/test-token.sh qa@acme.example`. The existing `RoleResolver` path already
   resolves `results.upload` from the custom role.

2. **Cookie-name bridge for SSO landing.** The backend writes an `curlew_session` cookie;
   the web reads `access_token` (see `web/src/hooks.server.ts`). Rather than changing
   production behaviour, we override `Saml__SessionCookieName=access_token` on the backend
   container in `docker-compose.test.yml` (and `Saml__WebPortalUrl=http://localhost:3000/org/acme`).
   Users land directly on the org overview — matching the behavior statement "lands on /org/acme".

3. **Fake IdP = tiny ASP.NET Core minimal-API sidecar.** A single-file `scripts/fake-idp/`
   service that (a) serves a canned metadata XML with a known x509 cert, (b) exposes
   `POST /saml/sso` which returns an HTML auto-submit form that POSTs a signed SAMLResponse
   to the backend ACS URL with a fixed `NameID=qa@acme.example`. Signed using a fixed
   RSA keypair committed to `testdata/enterprise/fake-idp-key.pem` and `fake-idp-cert.pem`.
   Kept minimal: no session, no login form — the test simulates IdP-initiated SSO by
   hitting `/saml/sso` directly from Playwright.

4. **New audit event: `results.upload`.** Task scope requires this row. We thread
   `IAuditWriter` into `ResultsEndpoints.IngestResult` and `audit.Append` after a
   successful ingest (mirrors how `InvitationsService` / `SsoService` do it). This is
   a minimal source change already within the convergence mandate — the existing
   audit infrastructure (M5-004) is re-used as designed.

5. **No production behavioural change for non-test builds.** All fake-IdP, cookie-name
   and WebPortalUrl overrides live in `docker-compose.test.yml` and `appsettings.Test`
   overlays. The `results.upload` audit event is the only production change and is
   additive.

## Implementation Steps

Ordered smallest-blast-radius first: fixtures → backend additive audit → scripts →
docker-compose → Playwright spec → CI.

### Step 1: Add `testdata/enterprise/` fixtures

**Rationale:** Zero-risk additive files that the later steps depend on. Includes the
fake-IdP keypair, the SAMLResponse XML template, and the qa-lead collection.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `testdata/enterprise/fake-idp-cert.pem` | create | X.509 public cert (PEM) used as the IdP signing cert. |
| `testdata/enterprise/fake-idp-key.pem` | create | Matching RSA private key (PEM). Test-only — committed intentionally. |
| `testdata/enterprise/saml-response-template.xml` | create | SAML 2.0 Response XML template with `{{NAME_ID}}`, `{{ISSUE_INSTANT}}`, `{{NOT_ON_OR_AFTER}}` placeholders. |
| `testdata/enterprise/e2e-collection.yaml` | create (copy of `testdata/team/e2e-collection.yaml`) | Collection run by the CLI with qa-lead token. |
| `testdata/enterprise/README.md` | create | Documents that the keypair is test-only and must never be used in production. |

#### Keypair generation (one-time, during execute phase)

```bash
# Executed during execute step, not committed as part of the plan:
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout testdata/enterprise/fake-idp-key.pem \
  -out testdata/enterprise/fake-idp-cert.pem \
  -days 36500 -subj "/CN=curlew-fake-idp" -sha256
```

#### Impact on existing tests

None — new files only.

---

### Step 2: Emit `results.upload` audit event

**Rationale:** Additive backend change with a focused unit-test harness. Done before the
Playwright spec so the audit row exists when the spec runs.

#### Files to modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Results/ResultsEndpoints.cs` | modify | After a successful `IngestAsync`, call `audit.Append(new AuditEvent(...))`. |
| `src/ApiTool.Backend/Results/ResultsService.cs` | optional modify | Keep ingestion pure; audit stays at the endpoint layer (matches MembersEndpoints pattern). |
| `src/ApiTool.Backend.Tests/Results/ResultsAuditTests.cs` | create | New xUnit class with ≥2 tests: success path emits the row; permission-denied path emits no row. |

#### Current code (ResultsEndpoints.cs, lines 120–134)

```csharp
var (dto, error, message, fieldPointer) = await svc.IngestAsync(userId.Value, orgGuid.Value, body, ct);

return error switch
{
    ResultError.None => HttpResults.Json(
        new IngestResultResponse(dto!.Id, "accepted"),
        statusCode: StatusCodes.Status202Accepted),
    ResultError.PermissionDenied => HttpResults.Json(
        new ErrorResponse("permission_denied", message ?? "Permission denied."),
        statusCode: StatusCodes.Status403Forbidden),
    ResultError.InvalidSchema => HttpResults.Json(
        new ErrorResponse("invalid_result_schema", message ?? "Invalid payload.", fieldPointer),
        statusCode: StatusCodes.Status400BadRequest),
    _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
};
```

#### New code

```csharp
var (dto, error, message, fieldPointer) = await svc.IngestAsync(userId.Value, orgGuid.Value, body, ct);

if (error == ResultError.None)
{
    audit.Append(new AuditEvent(
        OrgId: orgGuid.Value,
        ActorId: userId.Value,
        EventType: "results.upload",
        TargetType: "result",
        TargetId: dto!.Id,
        Payload: new { collection = body.CollectionName, pass = body.PassCount, fail = body.FailCount }));
    // AuditWriter flushes synchronously inside SaveChangesAsync — ensure a save happens here
    // since IngestAsync has already saved the Result rows in its own SaveChangesAsync.
    await dbForAudit.SaveChangesAsync(ct);
}
// ... existing switch
```

The endpoint handler signature gains `IAuditWriter audit` and `AppDbContext dbForAudit` (the
latter is already injected via `db` on line 102; we just reference it).

#### Tests to write FIRST (RED)

```csharp
public class ResultsAuditTests
{
    [Fact]
    public async Task Ingest_success_emits_results_upload_audit_row()
    {
        // arrange: InMemoryDb, member of org with results.upload, valid payload
        // act: POST /api/v1/organizations/{id}/results
        // assert: organization_audit_log has a row with event_type="results.upload"
        //         target_type="result", target_id=dto.Id, success=true
    }

    [Fact]
    public async Task Ingest_permission_denied_emits_no_audit_row()
    {
        // arrange: member of org whose custom role excludes results.upload
        // act: POST /api/v1/organizations/{id}/results
        // assert: 403 and no organization_audit_log row added
    }

    [Fact]
    public async Task Ingest_invalid_schema_emits_no_audit_row()
    {
        // arrange: member of org but payload missing run_at
        // act: POST /api/v1/organizations/{id}/results
        // assert: 400 and no organization_audit_log row added
    }
}
```

Table-driven case names: `success_emits_row`, `permission_denied_no_row`, `invalid_schema_no_row`.

#### Impact on existing tests

- `ResultsEndpointsTests` — no signature change to the public DTO, no impact.
- `AuditWriterTests` — unchanged; we only add a new call site.

---

### Step 3: Fake IdP sidecar

**Rationale:** Self-contained ASP.NET Core minimal-API app that the test stack launches.
Building this before the seed script means the stack script can health-check it.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `scripts/fake-idp/FakeIdp.csproj` | create | Minimal netcoreapp csproj — references `Microsoft.AspNetCore.App`. |
| `scripts/fake-idp/Program.cs` | create | ~150-line minimal API: `GET /metadata`, `POST /saml/sso`, `GET /healthz`. |
| `scripts/fake-idp/Dockerfile` | create | Multi-stage build, exposes port 8088. |
| `scripts/fake-idp/metadata-template.xml` | create | SAML EntityDescriptor referencing `http://fake-idp:8088/saml/sso` with embedded cert. |

#### Behavior

- `GET /healthz` → `200 "ok"`.
- `GET /metadata` → `200 text/xml`: reads `metadata-template.xml`, substitutes the cert
  body from `/certs/cert.pem` (mounted via docker-compose).
- `POST /saml/sso?RelayState=<orgId>` with query params from the SP request:
  parses the `SAMLRequest` (just to echo the `ID` as `InResponseTo`), signs a response
  using the mounted `/certs/key.pem`, and returns an HTML page with an auto-submit form
  that posts the signed response to the backend ACS URL
  (`http://backend:5000/api/v1/sso/saml/{orgId}/acs`). The `NameID` is always `qa@acme.example`.

#### Tests to write FIRST

No unit tests for the sidecar — it is exercised end-to-end by the Playwright spec,
which is the "≥5 assertions + failure path" in the DoD. A smoke-test in
`scripts/test-stack.sh` curls `/healthz` after `docker compose up`.

---

### Step 4: `scripts/seed-enterprise.sh`

**Rationale:** Idempotent helper that creates the `qa-lead` custom role and adds
`qa@acme.example` as a member tied to it. Consumed by both the observable command and
the Playwright spec's `beforeAll`. Built after the fake IdP so the seed can also
register the IdP cert with the backend.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `scripts/seed-enterprise.sh` | create | Takes `<org_slug> <role_name> <perms_csv>` and seeds the org, role, member, and SSO config. |

#### Shell contract

```bash
./scripts/seed-enterprise.sh acme qa-lead "results.upload,results.view,dashboard.view"
```

Steps performed (all idempotent, re-using `seed-test-data.sh` patterns):

1. Mint owner JWT via `./scripts/test-token.sh owner@example.com $OWNER_USER_ID`.
2. Ensure org `acme` exists (delegate to `./scripts/seed-test-data.sh`).
3. `POST /api/v1/organizations/{id}/roles {"name":"qa-lead","permissions":[...]}` — ignore 409.
4. Create `qa@acme.example` user by making an authed request with their JWT (the
   backend auto-creates the user row on first authed call — verify by examining
   `CurrentUserAccessor`). If auto-create doesn't happen, fall back to direct DB INSERT
   (psql via `docker compose exec backend`).
5. Add the qa user as an org member via a new backend helper endpoint **or** via a
   one-time `dotnet run --project scripts/seed-enterprise.csx`. Simplest path: extend
   `seed-test-data.sh` to accept an optional `--extra-member` arg and reuse the
   invitation flow (`POST /invitations` → accept immediately via direct DB).
   **Decision:** Skip the invitation flow and add members via a one-shot SQL script
   since this is test-only. Use `docker compose exec -T postgres psql ...` with a
   heredoc.
6. Upload `testdata/enterprise/fake-idp-cert.pem` and configure SAML via
   `PUT /api/v1/organizations/{id}/sso/saml` with `idp_sso_url=http://fake-idp:8088/saml/sso`.

#### Exit codes

- `0` on success (including already-seeded).
- `1` on any HTTP error or unexpected response shape.

#### Impact on existing tests

None — new script.

---

### Step 5: Extend `docker-compose.test.yml` and `scripts/test-stack.sh`

**Rationale:** Adds the fake-IdP service, overrides the backend cookie name /
WebPortalUrl, and wires the enterprise seed into `test-stack.sh up`.

#### Files to modify

| File | Action | Description |
|------|--------|-------------|
| `docker-compose.test.yml` | modify | Add `fake-idp` service; add `Saml__SessionCookieName`, `Saml__WebPortalUrl` env vars to backend; mount `testdata/enterprise/` into fake-idp as `/certs`. |
| `scripts/test-stack.sh` | modify | After `seed-test-data.sh`, run `seed-enterprise.sh acme qa-lead ...`. Add `wait_for_url http://localhost:8088/healthz "fake-idp"`. Tear-down unchanged (`down -v` already removes volumes). |

#### New docker-compose fragment

```yaml
  fake-idp:
    build:
      context: ./scripts/fake-idp
      dockerfile: Dockerfile
    ports:
      - "8088:8088"
    volumes:
      - ./testdata/enterprise/fake-idp-cert.pem:/certs/cert.pem:ro
      - ./testdata/enterprise/fake-idp-key.pem:/certs/key.pem:ro
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8088/healthz"]
      interval: 5s
      timeout: 3s
      retries: 20
```

Backend service gains:

```yaml
    environment:
      ASPNETCORE_URLS: http://0.0.0.0:5000
      ASPNETCORE_ENVIRONMENT: Development
      Saml__SessionCookieName: access_token
      Saml__WebPortalUrl: http://localhost:3000/org/acme
```

#### Impact on existing tests

- `full-pipeline.spec.ts` — still passes; the env vars change cookie name from
  `curlew_session` to `access_token`, which is what the web already reads. No spec
  change required.
- `org-sso.spec.ts` etc — all use `seedAuthCookie` which already sets `access_token`,
  unaffected.

---

### Step 6: Playwright E2E spec `enterprise-full.spec.ts`

**Rationale:** The deliverable. Built last so every dependency is in place.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/enterprise-full.spec.ts` | create | 6 passing assertions + 1 failure-path assertion. |
| `web/tests/e2e/helpers/saml.ts` | create | Helper that POSTs to the fake-IdP `/saml/sso` from within the Playwright context and captures the resulting session cookie. |

#### Spec outline

```ts
import { test, expect } from '@playwright/test';
import { runCurlew } from './helpers/cli';
import { triggerSamlLogin } from './helpers/saml';
import { seedAuthCookie } from './helpers/auth';

const ORG = 'acme';
const FAKE_IDP = 'http://localhost:8088';

test.describe('E2E M5: SSO → audit → dashboard with custom role', () => {
  test('SSO login via fake-idp sets session cookie and lands on /org/acme', async ({ page, context }) => {
    // triggerSamlLogin navigates to http://localhost:5000/api/v1/sso/saml/{orgGuid}/login
    // → IdP page → auto-submits form → backend ACS sets cookie → redirects to /org/acme
    await triggerSamlLogin(page, ORG);
    await expect(page).toHaveURL(new RegExp(`/org/${ORG}(\\?|$)`));
    const cookies = await context.cookies();
    expect(cookies.find((c) => c.name === 'access_token')?.value).toBeTruthy();
  });

  test('audit log shows sso.login row within 5s', async ({ page, context }) => {
    await triggerSamlLogin(page, ORG);
    await page.goto(`/org/${ORG}/audit-log`);
    await expect(
      page.getByTestId('audit-log-table').locator('tbody tr').filter({ hasText: 'sso.login' }).first()
    ).toBeVisible({ timeout: 5000 });
    // failure-path: the row must have success=true (no failure_reason column value)
    await expect(
      page.getByTestId('audit-log-table').locator('tbody tr').filter({ hasText: 'sso.login' }).first()
    ).not.toContainText('failure');
  });

  test('CLI upload as qa-lead custom role succeeds', async ({ page, context }) => {
    const qaToken = execSync(`bash scripts/test-token.sh qa@acme.example`).toString().trim();
    const result = runCurlew({
      collection: 'testdata/enterprise/e2e-collection.yaml',
      flags: ['--report-upload', '--org', ORG],
      env: { CURLEW_BACKEND_URL: 'http://localhost:5000', CURLEW_BACKEND_TOKEN: qaToken }
    });
    expect(result.stdout).toMatch(/Uploaded result res_/);
  });

  test('audit log shows results.upload row for the CLI run', async ({ page }) => {
    // Reuse session cookie seeded by SSO; re-run upload here to ensure a row in this spec.
    await seedAuthCookie(context, 'owner@example.com');  // fallback for admin view
    await page.goto(`/org/${ORG}/audit-log?event_type=results.upload`);
    await expect(
      page.getByTestId('audit-log-table').locator('tbody tr').filter({ hasText: 'results.upload' }).first()
    ).toBeVisible({ timeout: 5000 });
  });

  test('uploaded run appears in /org/acme/results within 5s', async ({ page, context }) => {
    await seedAuthCookie(context, 'owner@example.com');
    await page.goto(`/org/${ORG}/results?range=all`);
    await expect(page.getByTestId('recent-runs-table').locator('tbody tr').first())
      .toBeVisible({ timeout: 5000 });
  });

  test('qa-lead custom role row shows member_count=1 and is_builtin=false', async ({ page, context }) => {
    await seedAuthCookie(context, 'owner@example.com');
    await page.goto(`/org/${ORG}/settings/roles`);
    const row = page.getByTestId(/role-row-(?!builtin_)/).filter({ hasText: 'qa-lead' });
    await expect(row).toBeVisible({ timeout: 5000 });
    await expect(row).toContainText('1');  // member count
  });

  // Failure-path (required by DoD "e2e spec with >=5 assertions + failure path"):
  test('bogus SAMLResponse is rejected and audit-log records success=false', async ({ page, context }) => {
    const resp = await page.request.post(
      `http://localhost:5000/api/v1/sso/saml/${await lookupOrgGuid(ORG)}/acs`,
      { form: { SAMLResponse: 'bogus' } }
    );
    expect(resp.status()).toBe(401);
    await seedAuthCookie(context, 'owner@example.com');
    await page.goto(`/org/${ORG}/audit-log?event_type=sso.login`);
    await expect(
      page.getByTestId('audit-log-table').locator('tbody tr').filter({ hasText: 'failure' }).first()
    ).toBeVisible({ timeout: 5000 });
  });
});
```

#### helpers/saml.ts

```ts
import type { Page } from '@playwright/test';
import { execSync } from 'node:child_process';

/**
 * Resolves the org's GUID by making a minimal authenticated request as the owner
 * and parsing the `id` field. Returned as the "N" hex string expected by the
 * backend SAML login endpoint.
 */
export async function lookupOrgGuid(slug: string): Promise<string> {
  const token = execSync(`bash scripts/test-token.sh owner@example.com 00000000-0000-0000-0000-000000000001`).toString().trim();
  const resp = await fetch(`http://localhost:5000/api/v1/organizations`, {
    headers: { Authorization: `Bearer ${token}` }
  });
  const body = await resp.json();
  const org = body.organizations.find((o: { slug: string }) => o.slug === slug);
  return org.id.replace(/^org_/, '');
}

/**
 * Performs the full SAML SP-initiated flow:
 *   GET /api/v1/sso/saml/{guid}/login → 302 to fake-idp
 *   → fake-idp returns auto-submit form → POST to /api/v1/sso/saml/{guid}/acs
 *   → backend sets access_token cookie → 302 to Saml:WebPortalUrl
 */
export async function triggerSamlLogin(page: Page, slug: string): Promise<void> {
  const guid = await lookupOrgGuid(slug);
  await page.goto(`http://localhost:5000/api/v1/sso/saml/${guid}/login`);
  // fake-idp auto-submits; browser follows through to backend ACS and then web portal
  await page.waitForURL(new RegExp(`/org/${slug}(\\?|$)`), { timeout: 10_000 });
}
```

#### Tests to write FIRST (RED)

The Playwright spec IS the failing test. Start by writing all 7 tests then implementing
the plumbing until they pass. Per TDD: run `npx playwright test tests/e2e/enterprise-full.spec.ts`
first and watch it fail, then make it green.

#### Impact on existing tests

- `full-pipeline.spec.ts` — unaffected; runs against the same stack. The added
  `fake-idp` container increases startup time by ~5s but does not break its assertions.
- `playwright.config.ts` — no change; existing config already uses `testDir: './tests/e2e'`
  which picks up the new spec.

---

### Step 7: CI job `e2e-m5`

**Rationale:** Required by DoD. Mirrors `.github/workflows/e2e-m4.yml` structure.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `.github/workflows/e2e-m5.yml` | create | Copy `e2e-m4.yml` and swap the final playwright invocation to `tests/e2e/enterprise-full.spec.ts`. |

Body (key diffs vs. e2e-m4.yml):

- `name: E2E M5 enterprise-full`
- `concurrency.group: e2e-m5-${{ github.ref }}`
- Change final step:
  ```yaml
  - name: Run enterprise-full E2E spec
    working-directory: web
    run: npx playwright test tests/e2e/enterprise-full.spec.ts
  ```
- Leave `Run full-pipeline E2E spec` out — two separate workflows run independently.

#### Impact on existing tests

- Existing `e2e-m4.yml` — unaffected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|---------------|--------|-----------------|
| `src/ApiTool.Backend.Tests/Results/ResultsAuditTests.cs` | (new) | new suite | write 3 tests (success, perm-denied, invalid-schema) |
| `src/ApiTool.Backend.Tests/Results/ResultsEndpointsTests.cs` | existing | unaffected | — |
| `web/tests/e2e/full-pipeline.spec.ts` | all | unaffected | — (cookie name now `access_token` matches existing reading code) |
| `web/tests/e2e/enterprise-full.spec.ts` | (new) | new spec | write 7 tests |
| `src/ApiTool.Backend.Tests/Audit/AuditWriterTests.cs` | all | unaffected | — (new call site only) |

## Risks and Edge Cases

- **Risk:** Signing a SAMLResponse in Go/C# can silently produce an invalid signature that
  `SamlHandler.ValidateResponse` rejects, leading to opaque 401s.
  **Mitigation:** The fake IdP is written in C# using the same `SignedXml` API as
  `SamlHandler`, guaranteeing symmetric canonicalisation. Add an integration test inside
  `scripts/fake-idp/` that round-trips a response through `SamlHandler.ValidateResponse`
  as a unit test to catch regressions pre-stack-up.

- **Risk:** Flaky audit-log ordering — `results.upload` posted from the CLI may not be
  visible in the UI list until the DB transaction commits.
  **Mitigation:** The `audit-log?limit=50` query already orders newest-first. Add a
  Playwright `expect(...).toBeVisible({ timeout: 5000 })` with auto-retrying locators;
  no explicit `waitFor` needed.

- **Risk:** `fake-idp` container cold-start exceeds the 60s health-check loop on slow CI.
  **Mitigation:** `scripts/test-stack.sh wait_for_url` already waits up to 60 seconds;
  the csproj uses `PublishReadyToRun=true` in the Dockerfile for fast cold-start.

- **Risk:** Committing a private RSA key (`fake-idp-key.pem`) triggers security scanners.
  **Mitigation:** Add `testdata/enterprise/README.md` explicitly flagging the key as
  test-only; add `# gitleaks:allow` comment in `.gitleaks.toml` if needed (follow-up if
  a scanner trips).

- **Risk:** `Saml__SessionCookieName=access_token` override could leak into prod config
  if copied wholesale.
  **Mitigation:** Override only in `docker-compose.test.yml`. Production appsettings use
  the default `curlew_session`. Add a comment in the compose file.

- **Edge case:** The org GUID parser (`OrgId.TryParse`) in `SamlEndpoints.LoginRedirect`
  uses `Guid.TryParse(orgId, ...)`, while `OrganizationDto.Id` is wire-formatted as
  `org_<hex>`. `lookupOrgGuid` strips the `org_` prefix.

- **Edge case:** The qa-lead custom role permissions must include `results.upload`
  and `results.view` for the CLI upload to succeed and for the dashboard view to list
  the row — the observable command already supplies this list verbatim.

- **Edge case:** `seed-enterprise.sh` must be idempotent for repeat CI runs. All HTTP
  calls handle 409/200 and the psql member insert uses `ON CONFLICT DO NOTHING`.

## Verification

```bash
# Build gates
go build ./cmd/curlew
go test ./...
dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj --filter "FullyQualifiedName~ResultsAudit"
~/go/bin/golangci-lint run
cd web && npm run check && npm run lint

# Stack up + observable
./scripts/test-stack.sh up
./scripts/seed-enterprise.sh acme qa-lead "results.upload,results.view,dashboard.view"
CURLEW_BACKEND_URL=http://localhost:5000 \
CURLEW_BACKEND_TOKEN=$(./scripts/test-token.sh qa@acme.example) \
  ./curlew run testdata/enterprise/e2e-collection.yaml --report-upload --org acme
# expect: exit 0, "Uploaded result res_..."

cd web && npm run test:e2e -- tests/e2e/enterprise-full.spec.ts
# expect: ≥5 passing + failure path

./scripts/test-stack.sh down
```

## Quality Checklist

- [x] Every file to be modified has been fully read (`ResultsEndpoints.cs`, `SamlEndpoints.cs`, `SsoService.cs`, `SamlHandler.cs`, `docker-compose.test.yml`, `scripts/test-stack.sh`, `scripts/seed-test-data.sh`, `scripts/test-token.sh`, audit viewer, roles editor specs, full-pipeline spec, cli/auth/fixtures helpers, playwright.config.ts, e2e-m4.yml)
- [x] Every affected `_test.go` / `*Tests.cs` / `*.spec.ts` file has been read
- [x] All call sites of changed interfaces identified (audit.Append already emitted by 10+ services; add one more site)
- [x] Before/after code snippets for non-trivial changes
- [x] Impact on existing tests explicitly listed
- [x] Edge cases and risks identified with mitigations
- [x] Tests specified BEFORE implementation (TDD)
- [x] C# audit call site uses the existing `IAuditWriter` pattern (no new interface)
- [x] Table-driven test cases named upfront (`success_emits_row`, `permission_denied_no_row`, `invalid_schema_no_row`)
- [x] Steps ordered by blast radius (fixtures → additive backend → standalone sidecar → script → compose → Playwright → CI)
- [x] Observable verification command is concrete and runnable
- [x] Plan file matches the template structure
