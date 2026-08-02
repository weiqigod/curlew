# Improvement Report: M5-020

**Task:** E2E: SSO login -> audit capture -> dashboard with custom role
**Date:** 2026-04-19
**Review:** management/reviews/M5-020-review.md

## Resolved Findings

### Iteration 1 (commit 5415aac)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Behavior 6 (`is_builtin=false`) not asserted — locator used generic `[data-testid]` filter with no assertion to distinguish custom vs built-in roles | Changed locator to `page.getByTestId(/^role-row-role_/)` (selects only custom role rows) and added `.not.toContainText('Built-in')` assertion | ✓ tests pass |
| 2 | Low | `testdata/enterprise/saml-response-template.xml` orphaned — committed and documented in README but never read by `Program.cs` which builds SAML responses inline | Deleted the file and removed its row from `testdata/enterprise/README.md` | ✓ tests pass |
| 3 | Low | Behavior 4 (actor display name) not asserted — `uploadRow` only checked for `results.upload` text, not the qa user's email | Added `.toContainText('qa@acme.example')` assertion on the `uploadRow` locator | ✓ tests pass |
| 4 | Low | `orgGuid` from `RelayState` query param interpolated into HTML without validation or HTML encoding — could corrupt form if value contains `"` or `>` | Added `Guid.TryParse` validation (returns 400 if invalid) and applied `System.Net.WebUtility.HtmlEncode()` to all HTML attribute interpolations | ✓ tests pass |

### Iteration 2 (commit 370dc02)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 5 | Low | Assertion 7 ("audit-log records success=false") only checked `toBeVisible()` on the `sso.login` row — never verified the row indicated failure, making the test name unsubstantiated | Added `await expect(failRow).toContainText('failure')` after the `toBeVisible()` call so failure state is actually asserted | ✓ tests pass |
| 6 | Low | `RSA.Create()` in `scripts/fake-idp/Program.cs` was not wrapped in `using`, leaking a native RSA key handle on every `/saml/sso` call (CA2000) | Changed `var rsa = RSA.Create();` to `using var rsa = RSA.Create();` | ✓ tests pass |

### Iteration 3 (commit d5366c9)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 7 | Low | Assertion 2 (`sso.login` audit row) checked visibility and absence of 'failure' but never asserted actor email (`qa@acme.example`) — Behavior 2 required identity verification, parallel omission to the results.upload row fixed in iteration 1 | Added `await expect(ssoLoginRow).toContainText('qa@acme.example');` between the `toBeVisible` and `not.toContainText('failure')` assertions | ✓ tests pass |
| 8 | Low | Steps 6 (assign custom role) and 7 (configure SAML) in `seed-enterprise.sh` emitted `WARN` on non-200 HTTP and continued — silently partial-seeded stacks caused confusing Playwright failures instead of a clear error | Changed both `WARN` blocks to `echo "ERROR: ..."` + `exit 1` so the seed script fails fast on either critical step failure | ✓ tests pass |

### Iteration 4 (commit df88bb1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 9 | Low | Step 3 (subscription checkout) in `seed-enterprise.sh` captured `$SUB_HTTP` and printed it but never validated the status code. A checkout failure (400/500) silently continued, causing the downstream invitation step to fail with a misleading "seat limit reached" error | Added HTTP 200/201 guard after `echo " Subscription checkout → HTTP $SUB_HTTP"`, matching the validated pattern used in steps 4, 6, and 7 | ✓ tests pass |

### Iteration 5 (commit 1537dc2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 10 | Medium | `fake-idp` Docker healthcheck in `docker-compose.test.yml` used `wget`, which is absent from the `mcr.microsoft.com/dotnet/aspnet:9.0` Debian-based image (unlike the backend which uses Alpine + explicit `apk add wget`). Docker perpetually marked the container as "unhealthy", misleading operators and creating a latent breakage point for any future `depends_on: condition: service_healthy` | Changed healthcheck in `docker-compose.test.yml` from `["CMD", "wget", "-qO-", ...]` to `["CMD", "curl", "-fsS", ...]` — curl is present in the Debian aspnet image | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 86.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 5415aac | fix(e2e): resolve review findings for M5-020 | #1, #2, #3, #4 |
| 370dc02 | fix(e2e): add failure-state assertion and dispose RSA handle | #5, #6 |
| d5366c9 | fix(e2e): add sso.login email assertion and harden seed-enterprise exit codes | #7, #8 |
| df88bb1 | fix(scripts): validate subscription checkout HTTP status in seed-enterprise.sh | #9 |
| 1537dc2 | fix(docker): use curl for fake-idp healthcheck instead of wget | #10 |

## Summary
10/10 findings resolved across 5 review iterations. 0 deferred.
