# Improvement Report: M4-003

**Task:** Backend: organization + seat RBAC data model and service
**Date:** 2026-04-15
**Review:** management/reviews/M4-003-review.md

---

## Round 3 (post-third-review fix)

### Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `trimmedName.Length > 100` branch in `CreateAsync` had no test coverage — missing boundary tests analogous to slug's `Rejects_slug_exceeding_max_length` / `Accepts_slug_at_max_length` | Added `Create_rejects_name_exceeding_max_length` (101-char name → `OrgError.InvalidName`) and `Create_accepts_name_at_max_length` (100-char name → success), mirroring the SlugValidator boundary test pattern | ✓ tests pass |

### Out of Scope (Deferred)

No findings deferred. All findings resolved.

### Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS |
| `dotnet test` (all tests) | PASS — 48/48 |
| `dotnet test --filter "FullyQualifiedName~Organizations\|FullyQualifiedName~Rbac"` | PASS — 43/43 (≥12 DoD) |

### Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `9b2913c` | test(organizations): add name max-length boundary tests | #1 |

### Summary

1/1 findings resolved. 0 deferred. Test count increased from 46 to 48.

---

## Round 2 (findings from second review — Low findings)

### Resolved Findings (this round — Low findings from second review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `_resolved` cache in `CurrentUserAccessor` is dead code — scoped lifetime means the early-return guard at lines 27-28 is never reached; test claimed to cover cache-hit but was actually testing "user already in DB on 2nd request" | Removed `_resolved` field, early-return guard, and trailing assignment; renamed test to `ResolveAsync_succeeds_on_second_request_when_user_row_already_exists` with accurate description | ✓ tests pass |
| 2 | Low | `DbUpdateException` catch block in `CreateAsync` has 0% coverage — the concurrent-slug race path cannot be triggered by in-memory SQLite (single connection serialises writes) | Added explanatory comment to the catch block documenting that this is the concurrent-insert safety net and why automated test coverage is not feasible with current infrastructure; exposed `TestDbScope.Connection` as internal for potential future use | ✓ tests pass |
| 3 | Low | `CreateOrganizationRequest.Name` had no server-side validation — empty/whitespace name produced a 201 with a blank display name | Added `OrgError.InvalidName` enum member; added name guard + trim at top of `CreateAsync` (before slug validation); mapped to HTTP 400 `invalid_name` in endpoint switch; used `trimmedName` for both `org.Name` and audit-log payload; added 2 service tests + 1 endpoint test (TDD) | ✓ tests pass |

### Out of Scope (Deferred)

No findings deferred. All findings resolved.

### Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS |
| `dotnet test` (all tests) | PASS — 46/46 |
| `dotnet test --filter "FullyQualifiedName~Organizations\|FullyQualifiedName~Rbac"` | PASS — 41/41 (≥12 DoD) |
| `golangci-lint run` (Go side) | PASS — 0 issues |
| Coverage | ≥90% line (structural-only changes for #1/#2; #3 adds covered paths) |

### Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `4f63c0b` | fix(backend): validate organization name is non-empty | #3 |
| `fcfa134` | fix(backend): remove unreachable _resolved cache from CurrentUserAccessor | #1 |
| `490f17b` | fix(backend): document DbUpdateException catch as untestable concurrent safety net | #2 |

### Summary

3/3 findings resolved. 0 deferred. Test count increased from 43 to 46 (3 new tests for finding #3).

---

## Round 1 (findings from first review — all resolved)

| # | Severity | Finding | Fix Applied |
|---|----------|---------|------------|
| 1 | High | Non-GUID `sub` claim → 500 instead of 401 | `ResolveAsync` returns `Guid?`; endpoint handlers return 401 on null |
| 2 | High | Audit log JSON via string interpolation — broken for `"` in names | Switched to `JsonSerializer.Serialize` |
| 3 | Medium | Slug race condition with no `DbUpdateException` handler | Added `try/catch(DbUpdateException)` with `IsUniqueConstraintViolation` |
| 4 | Medium | `SqliteConnection` leak in `TestDb.CreateOpen()` | Introduced `TestDbScope` (owns both connection and context) |
| 5 | Medium | `BackendFactory` uses EF InMemory (no unique constraints) | Added comment explaining two-layer test strategy |
| 6 | Medium | N+1 `CountAsync` in `ListForUserAsync` | Replaced with correlated subquery in single LINQ join |
| 7 | Medium | SQL injection in `TableExistsAsync` test helper | Replaced interpolation with parameterized query |
| 8 | Low | No slug boundary tests (100-char max) | Added `Rejects_slug_exceeding_max_length` + `Accepts_slug_at_max_length` |
| 9 | Low | No test for the `_resolved` cache path | Added test (subsequently corrected in this round) |
