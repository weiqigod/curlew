# Improvement Report: M14-003

**Task:** Backend: GET /api/v1/.well-known/jwks.json
**Date:** 2026-05-04
**Review:** management/reviews/M14-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | CHANGELOG.md not updated — no M14-003 entry | Added entry at top of `### Added` section in CHANGELOG.md documenting the JWKS endpoint, RFC 8615 well-known URI, Cache-Control: public max-age=3600, unauthenticated design, and AllowAnonymous behaviour | ✓ tests pass |
| 2 | High | Task YAML status was `backlog` instead of `review` | Changed `status: backlog` to `status: review` in `management/tasks/M14-003.yaml` | ✓ tests pass |
| 3 | Medium | Stale class doc comment in `SwaggerAuthSurfaceTests.cs` — said "login endpoint" while the class now tests login, refresh, and JWKS | Updated summary to "Verifies that Swagger correctly exposes the auth login, refresh, and JWKS endpoints." | ✓ tests pass |
| 4 | Medium | DoD item "OpenAPI surface lists the endpoint as unauthenticated and JSON-typed" not fully tested — Swagger test didn't assert (a) no Bearer security block and (b) response content-type is `application/jwk-set+json` | (a) Changed `.Produces(StatusCodes.Status200OK, contentType: "application/jwk-set+json")` to `.Produces<System.Text.Json.JsonElement>(StatusCodes.Status200OK, contentType: "application/jwk-set+json")` so the content type appears in the OpenAPI 200 response block. (b) Added two assertions to `Swagger_lists_jwks_endpoint_and_NOT_v4_1_public_key`: `jwksGet.TryGetProperty("security", out _).Should().BeFalse()` (verifies AllowAnonymous strips global Bearer requirement) and content-type `application/jwk-set+json` is present in the 200 response content block | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Coverage (JwksEndpoint.cs) | 100% line, 100% branch |
| Total test count | 713 passed, 0 failed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| bd9417c6 | fix(auth): complete DoD for JWKS endpoint OpenAPI surface | #2, #3, #4 |
| 756a0281 | fix(changelog): add M14-003 JWKS endpoint entry | #1 |

## Summary

4/4 findings resolved. 0 deferred.

Key fix: the non-generic `.Produces()` call did not emit content-type information into the OpenAPI specification. Switching to the generic `.Produces<JsonElement>()` form caused Swashbuckle to include the `application/jwk-set+json` content block in the 200 response schema — aligning the OpenAPI surface with the runtime Content-Type header already being set correctly. The security assertion confirmed that Swashbuckle automatically strips the global Bearer security requirement for `AllowAnonymous` endpoints, so no operation filter was needed.
