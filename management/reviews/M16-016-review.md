# Code Review: M16-016 (iteration 7)

**Task:** Web /integrations/gitlab dashboard page with PAT submission and project lookup
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-016-web-gitlab-integrations-page

## Verdict: PASS

## Findings

No findings.

## Prior Findings — All Resolved

The three findings from iteration 6 are all confirmed fixed:

1. **Medium — UriFormatException → 500 (finding #1):** `GitLabProjectLookup.cs` line 52 now uses `Uri.TryCreate(...)` to guard against invalid base URLs, returning `GitLabProjectLookupStatus.InvalidBaseUrl` instead of throwing. The endpoint maps this to `BadRequest400("invalid_base_url", ...)` at line 183–185. Both a unit test (`Invalid_base_url_returns_InvalidBaseUrl_instead_of_throwing`) and an endpoint test (`POST_lookup_InvalidBaseUrl_returns_400_invalid_base_url`) cover the path. A new `InvalidBaseUrl` enum member with doc comment is added to the public interface.

2. **Low — 403 branch untested (finding #2):** `Maps_403_to_Unauthorized` test added at line 181 of `GitLabProjectLookupTests.cs`. Asserts `GitLabProjectLookupStatus.Unauthorized` for `HttpStatusCode.Forbidden`.

3. **Low — 5xx branch untested (finding #3):** `Maps_5xx_to_Unreachable` test added at line 193 of `GitLabProjectLookupTests.cs`. Asserts `GitLabProjectLookupStatus.Unreachable` for `HttpStatusCode.ServiceUnavailable`.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error returns use `fmt.Errorf`-equivalent wrapping (C# exception pattern). `catch (DbUpdateException ex) when (IsUniqueConstraintViolation(ex))` guard prevents spurious 500s on duplicate insert. Tier-gate errors → 402 ProblemDetails. Lookup errors → typed 4xx/502. `JsonException` → `InvalidResponse`. Non-constraint `SaveChangesAsync` failures propagate to global 500 handler (correct behaviour). |
| Input Validation | PASS | `project_path` and `access_token` empty guards present. Invalid `gitlab_base_url` (syntactically bad URI) now returns `InvalidBaseUrl` → 400 instead of 500. `%`-encoded path rejected client-side in modal with helper text. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `IGitLabProjectLookup` interface correctly named. No package naming issues. |
| Code Organization | PASS | `LookupWithCaBundleAsync` correctly extracted as private static helper. `using` on all `HttpResponseMessage`, `HttpClient`, and `X509Certificate2` resources. `ResponseContentRead` ensures body buffered before disposal. `EncodeProjectPath` private helper encapsulates URL encoding. Single responsibility per type. No circular dependencies. |
| Correctness | PASS | `AnyAsync` duplicate guard uses numeric `project_id` aligned with DB partial-unique index. Cross-tenant isolation via `org_id` filter on DELETE (returns 404, not 403, to avoid leaking existence). PAT never echoed (no `AccessToken` field on `GitLabInstallationListItem`). LEFT JOIN `last_status_post_at` with nullable handling. `caBundleHandlerFactory` test seam prevents real X509 work in unit tests. Re-paste DELETE-then-POST flow consistent with plan. `IsUniqueConstraintViolation` guard matches `OrganizationService` established pattern. |
| Test Quality | PASS | All 8 task YAML behaviors covered by backend integration tests + E2E specs. `GitLabProjectLookupTests`: 15 cases (URL encoding ×4, Private-Token header, 200 ok, 200 missing id, 200 string id, 200 malformed JSON, 401, 404, network error, http AllowHttp=false, http AllowHttp=true, invalid base URL ×3, 403, 5xx, CA-bundle ok, CA-bundle network error). `GitLabIntegrationsEndpointsTests`: 20 cases covering all verbs, tier-gate, multi-org, duplicate, CA-bundle, revoked-token serialization, and `InvalidBaseUrl`. Web unit: 4 cases. Web E2E: 8 specs. |

## Test Coverage
- Go coverage: 87.1% (cmd/curlew) — exceeds 80% gate. No Go files changed in this task.
- C# backend: all `GitLabProjectLookupTests` and `GitLabIntegrationsEndpointsTests` pass. All previously identified code branches are now covered.
- Web unit (`gitlab-integrations.test.ts`): 4 cases covering list, create, remove, and 402 propagation with `org_id` forwarding.
- Web E2E (`org-integrations-gitlab.spec.ts`): 8 specs, one per task YAML behavior.

## Summary

All three findings from iteration 6 are resolved. The `InvalidBaseUrl` guard in `GitLabProjectLookup.cs` eliminates the unhandled `UriFormatException` path, and the two missing test cases for 403 and 5xx branches are now present. The codebase meets all error handling, input validation, naming, organisation, correctness, and test-quality standards. No new findings identified.
