# Code Review: M4-003

**Task:** Backend: organization + seat RBAC data model and service
**Reviewer:** AI
**Date:** 2026-04-15
**Branch:** feature/M4-003-backend-org-rbac
**Round:** 4 (post-round-3-improvement re-review)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All expected failures return errors (never panic/throw). Name null-coalescing + empty/overlength guards return `OrgError.InvalidName`. Slug validation returns `OrgError.InvalidSlug`. `DbUpdateException` in `CreateAsync` documented as untestable concurrent safety net; broad catch in `CurrentUserAccessor` documented as upsert race guard. Non-GUID `sub` returns null → 401. No swallowed errors. |
| Input Validation | PASS | Name: null/empty/whitespace/> 100 all rejected with `OrgError.InvalidName`. Slug: format/charset/length validated by `SlugValidator`. `OrgId.TryParse` rejects malformed ids → 404. Non-GUID sub → 401. `RequireAuthorization()` group-level auth runs before handler logic. |
| Naming | PASS | All exported symbols have doc comments. No stuttering (`OrgError`, `OrgRole`, `OrgStatus` use consistent short prefix). Short names in tight scopes. |
| Code Organization | PASS | Auth/Data/Organizations cleanly separated. `internal` modifiers applied correctly (`TestDb`, `TestDbScope`). `await using` resource cleanup throughout tests. No circular dependencies. |
| Correctness | PASS | `trimmedName` used consistently for org entity and audit log payload. `seat_count` computed from live DB membership count (not stored). `CancellationToken` propagated through every async call. `BackendCollection` + assembly-level `DisableTestParallelization` prevents SQLite concurrency conflicts. |
| Test Quality | PASS | Round 3 finding resolved: `Create_rejects_name_exceeding_max_length` (101 chars → `OrgError.InvalidName`) and `Create_accepts_name_at_max_length` (100 chars → success) both present and passing. All 8 task behaviors covered by named tests. Error paths, boundary values, and edge cases covered. Table-driven tests for slug validation. |

## Resolved Findings (all rounds)

| # | Round | Previous Finding | Status |
|---|-------|-----------------|--------|
| R1 | 3 | `trimmedName.Length > 100` branch in `CreateAsync` had no test coverage | ✓ `Create_rejects_name_exceeding_max_length` + `Create_accepts_name_at_max_length` added |
| R2 | 2 | `_resolved` dead-code cache in `CurrentUserAccessor` | ✓ Field removed; test description corrected |
| R3 | 2 | `DbUpdateException` catch block untested | ✓ Explanatory comment added; `Connection` exposed as internal |
| R4 | 2 | `CreateOrganizationRequest.Name` not validated | ✓ Guard + trim added; `OrgError.InvalidName` mapped to HTTP 400 |
| R5 | 1 | Non-GUID `sub` → 500 instead of 401 | ✓ `ResolveAsync` returns `Guid?`; endpoints return 401 on null |
| R6 | 1 | Audit log JSON via string interpolation | ✓ Switched to `JsonSerializer.Serialize` |
| R7 | 1 | Slug race condition with no `DbUpdateException` handler | ✓ `try/catch(DbUpdateException)` with `IsUniqueConstraintViolation` added |
| R8 | 1 | `SqliteConnection` leak in `TestDb.CreateOpen()` | ✓ `TestDbScope` owns and disposes both |
| R9 | 1 | N+1 `CountAsync` in `ListForUserAsync` | ✓ Resolved via correlated subquery in single LINQ join |
| R10 | 1 | SQL injection in `TableExistsAsync` test helper | ✓ Replaced with parameterised query |

## Behaviors Coverage

| # | Behavior | Test |
|---|----------|------|
| 1 | POST creates org → 201, role=owner, seat_count=1, seat_limit=10 | `Post_creates_org_and_returns_201_with_owner_role_and_default_seat_counts` |
| 2 | Duplicate slug → 409 `organization_slug_taken` | `Post_with_duplicate_slug_returns_409_organization_slug_taken` |
| 3 | Uppercase slug → 400 `invalid_slug` | `Post_with_uppercase_slug_returns_400_invalid_slug` |
| 4 | No memberships → GET list 200, empty array | `Get_list_returns_empty_for_user_with_no_memberships` |
| 5 | Owner GET detail → 200, role=owner, seat_count/seat_limit | `Get_detail_returns_200_with_role_and_seats_for_member` |
| 6 | Non-member GET detail → 404 `organization_not_found` (no enumeration) | `Get_detail_returns_404_for_non_member_without_leaking_existence` |
| 7 | Unauthenticated request → 401 `unauthorized` | `All_endpoints_return_401_when_no_bearer_header` |
| 8 | Migration creates all 4 tables on startup | `Migration_creates_expected_table` (4 variants) |

## Test Coverage

- **Total tests:** 48 (all pass)
- **Org/RBAC filter (`FullyQualifiedName~Organizations|FullyQualifiedName~Rbac`):** 43 (DoD ≥12 satisfied)
- **All 8 task behaviors:** covered by named tests
- **Intentionally uncovered (documented):**
  - `CreateAsync` — `DbUpdateException` catch block (concurrent-insert race; untestable with single-connection SQLite)
  - `OrganizationsEndpoints` — `_ => Results.StatusCode(500)` fallback arms (unreachable by design)
  - `Program.cs` ~41% — Development-only paths (Swagger, auto-migrate) not reachable via `WebApplicationFactory` (acceptable)
  - `OrganizationInvitation.cs` 0% — shell entity with no behaviour in this slice (acceptable)

## Summary

All findings from the three previous review rounds have been resolved. The round 3 Low finding (missing boundary tests for the 100-character name limit) was addressed by commit `9b2913c`, which adds both the over-limit test (101 chars → `OrgError.InvalidName`) and the at-limit test (100 chars → success), mirroring the existing `SlugValidator` boundary test pattern. 48/48 tests pass; 43/43 pass the DoD filter. No new findings.
