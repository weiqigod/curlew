# Code Review: M5-020 (Iteration 6)

**Task:** E2E: SSO login -> audit capture -> dashboard with custom role
**Reviewer:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-020-e2e-enterprise-full

## Verdict: PASS

## Context

This is the sixth review iteration. The single finding from iteration 5 was addressed in
commit `1537dc2`:

- Finding #10 (iter 5): `fake-idp` Docker healthcheck used `wget` (absent from
  `mcr.microsoft.com/dotnet/aspnet:9.0` Debian-based image) → changed to
  `["CMD", "curl", "-fsS", "http://localhost:8088/healthz"]` in `docker-compose.test.yml`.
  Confirmed correct at line 53 of the current file.

No new findings in this iteration.

## Findings

*(none)*

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All C# error paths return structured responses. `audit.Append` + `SaveChangesAsync` correctly guarded by `error == ResultError.None`. `body?.CollectionName` null-conditional is safe because `IngestAsync` only returns `ResultError.None` when the payload passes `ValidateRequest`, which validates `body != null`. Fake IdP validates `RelayState` GUID (returns 400) and key/cert mount existence (returns 500). Shell scripts use `set -euo pipefail` and validate all HTTP status codes at every step. |
| Input Validation | PASS | `IngestResult` validates Content-Length, JSON parsing, `userId`, `orgGuid`, and RBAC permission before `IngestAsync`. `IngestAsync` re-validates membership and schema. Shell scripts validate all three positional args with `${N:?...}`. Org slug lookup exits 1 if not found. All curl responses checked for expected HTTP status codes. |
| Naming | PASS | All exported C# symbols (`ResultsEndpoints`, `IngestResultResponse`, `ListResultsResponse`) have doc comments. No stuttering. TypeScript helpers (`lookupOrgGuid`, `triggerSamlLogin`, `seedAuthCookie`, `runApitest`) are clearly named with JSDoc comments. Package/file names follow conventions. |
| Code Organization | PASS | Audit event stays at the endpoint layer (consistent with `InvitationsService`, `SsoService` patterns). `IAuditWriter.Append` interface contract respected. Fake IdP is a self-contained sidecar with no cross-service imports. `saml.ts` helper is isolated in `helpers/`. No circular dependencies. `internal/` package boundaries unaffected (no Go changes). |
| Correctness | PASS | `using var rsa = RSA.Create()` correctly disposes RSA handle. `db.SaveChangesAsync(ct)` called after `audit.Append`. `Guid.TryParse` + `HtmlEncode` applied to all HTML attribute interpolations in `Program.cs`. Docker healthcheck for fake-idp uses `curl` (present in Debian aspnet image). `seed-enterprise.sh` is idempotent: 409 ignored for role creation, member check skips invite if already present, HTTP guard at every critical step. `test-stack.sh down` runs `down -v` to remove volumes. APITEST_BACKEND_TOKEN is set via `$GITHUB_ENV` before the Playwright run step in CI — all tests will execute, not skip. |
| Test Quality | PASS | All 7 behaviors from the task YAML covered by at least one test. Three-test C# `ResultsAuditTests` covers success, permission-denied (outsider), and invalid-schema paths. Seven Playwright tests (6 happy-path + 1 failure-path) meet DoD requirement of "≥5 assertions + failure path". Assertion 2 asserts actor email (`qa@acme.example`). Assertion 4 asserts actor email on upload row. Assertion 6 uses `getByTestId(/^role-row-role_/)` to select only custom roles and asserts `.not.toContainText('Built-in')` for `is_builtin=false`. Assertion 7 (failure-path) verifies both 401 status code and `toContainText('failure')` on the audit row. |

## Resolved Findings from All Prior Iterations

All ten prior findings confirmed resolved in current code:
- Finding #1 (iter 1): `is_builtin=false` locator → `page.getByTestId(/^role-row-role_/)` + `not.toContainText('Built-in')` ✓
- Finding #2 (iter 1): Orphaned `saml-response-template.xml` deleted ✓
- Finding #3 (iter 1): `toContainText('qa@acme.example')` on `results.upload` row (line 124) ✓
- Finding #4 (iter 1): `Guid.TryParse` + `HtmlEncode` applied to all HTML attribute interpolations in `Program.cs` ✓
- Finding #5 (iter 2): `await expect(failRow).toContainText('failure')` added (line 190) ✓
- Finding #6 (iter 2): `using var rsa = RSA.Create()` at line 175 ✓
- Finding #7 (iter 3): `await expect(ssoLoginRow).toContainText('qa@acme.example')` at line 61 ✓
- Finding #8 (iter 3): Steps 6 and 7 in `seed-enterprise.sh` now `exit 1` on non-200 ✓
- Finding #9 (iter 4): Step 3 subscription checkout HTTP status validated at lines 75–79 ✓
- Finding #10 (iter 5): `fake-idp` healthcheck changed from `wget` to `curl` in `docker-compose.test.yml` ✓

## Test Coverage

- **C# backend (`ResultsAuditTests`):** 3 tests — success path emits row, permission-denied
  emits no row, invalid-schema emits no row. All task audit behaviors covered.
- **Playwright E2E (`enterprise-full.spec.ts`):** 7 tests (6 happy-path + 1 failure-path).
  Meets DoD requirement (≥5 assertions + failure path). All 7 behaviors from the task YAML
  are individually verified by at least one assertion.
- **Build gates:** `go build` PASS, `go test ./...` PASS, `golangci-lint` 0 issues,
  `dotnet test` PASS (per improved.md quality gate table). Coverage 86.7%.

## Summary

All ten findings from iterations 1–5 are confirmed resolved in the current code. The iteration-5
fix (replacing `wget` with `curl` in the fake-idp Docker healthcheck) is correctly applied at
line 53 of `docker-compose.test.yml`. No new issues were identified. The implementation correctly
threads the `results.upload` audit event through the endpoint layer, the fake SAML IdP sidecar
is correctly implemented with RSA handle cleanup and input validation, and the Playwright spec
covers all 7 task behaviors with precise assertions.
