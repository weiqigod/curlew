# Verification Report: M5-006

**Task:** Backend: custom roles and granular permissions
**Verified by:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-006-custom-roles-permissions
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `dotnet build` | PASS | 0 warnings, 0 errors |
| `dotnet test ./...` | PASS | 484 tests, 0 failures, ~8 s |
| Observable filter `~Rbac&~CustomRoles` | PASS | 37 tests (threshold ≥12 met) |
| Coverage (line-rate) | 92% | Meets >= 80% threshold |
| `golangci-lint run` | N/A | C# project — no Go linter applicable |

## Observable Output

```
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
    --filter "FullyQualifiedName~Rbac&FullyQualifiedName~CustomRoles"

Passed!  - Failed: 0, Passed: 37, Skipped: 0, Total: 37, Duration: 1 s
```

Expected: Passed >= 12, Failed: 0
Result: MATCH (37 passed)

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | POST /organizations/{id}/roles creates role with 201 and id role_... | `Post_creates_custom_role_and_returns_201_with_role_id` | PASS |
| 2 | Duplicate role name in same org → 409 role_name_taken | `Post_duplicate_name_returns_409_role_name_taken` | PASS |
| 3 | Unknown permission key → 400 invalid_permission with offending key | `Post_unknown_permission_key_returns_400_invalid_permission_with_field` | PASS |
| 4 | GET /roles returns 3 built-ins (is_builtin=true) + custom role | `Post_then_Get_list_includes_builtins_and_custom` | PASS |
| 5 | Custom-role member passes results.upload gate, fails members.invite gate | `Custom_role_member_passes_results_upload_gate_and_fails_members_invite_gate` | PASS |
| 6 | DELETE blocks with 409 role_in_use + member_count when members reference role | `Delete_when_members_reference_role_returns_409_role_in_use_with_count` | PASS |
| 7 | Non-owner POST → 403 permission_denied | `Post_by_admin_returns_403_permission_denied` | PASS |
| 8 | PATCH /members/{id} with role_id writes role.changed audit row | `Patch_member_with_role_id_writes_role_changed_audit_event` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 484 total tests, 37 Rbac+CustomRoles tests, 0 failures | PASS |
| 2 | Observable output works as specified | 37 tests with filter `~Rbac&~CustomRoles` (threshold ≥12) | PASS |
| 3 | Test coverage >= 80% | line-rate=0.92 (92%) from cobertura coverage report | PASS |
| 4 | No build warnings or lint errors | `dotnet build` 0 warnings 0 errors | PASS |
| 5 | Swagger lists POST/GET/DELETE /organizations/{id}/roles endpoints | `CustomRolesEndpoints.cs` maps all three routes with `WithOpenApi()` | PASS |
| 6 | Smoke test or equivalent integration check updated | 15 integration tests in `CustomRolesEndpointsTests` covering all behaviors | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — tuple returns with `CustomRoleError` enum; no swallowed exceptions; `JsonException` caught in `ToDto`/`DeserializeKeys` with safe defaults |
| Input validation | PASS — name trimmed, empty/reserved/too-long rejected; unknown permission keys rejected with offending key |
| Naming conventions | PASS — no stuttering; all exported types and methods have doc comments |
| Code organization | PASS — `Rbac/CustomRoles/` folder, single responsibility per file, DI-registered in `Program.cs` |
| Test quality | PASS — table-driven tests used; integration tests via `BackendFactory`; unit tests via `TestDb` |
| Effective permissions | PASS — `RoleResolver` correctly computes custom role ∪ direct overrides, falls back to built-in for deleted/cross-org role |

Branch A: Review PASS (iteration 3) trusted, spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| 8b27a7f | fix(rbac): return OrganizationNotFound when member is null in CustomRolesService |
| 028cb81 | docs(review): add passing review for M5-006 (iteration 3) |
| 44dd16b | docs(review): update improvement report for M5-006 iteration 2 |
| fc7a59f | fix(rbac): remove dead code, fix comment, add seats.remove test |
| cd9eaf4 | docs(review): add review with findings for M5-006 (iteration 2) |
| a531f11 | docs(review): add improvement report for M5-006 |
| 9da5d76 | fix(rbac): gate invitations on RoleResolver permission check (members.invite) |
| 6c37881 | fix(rbac): correct BuiltInMember and BuiltInAdmin permission sets per spec |
| f54f9bf | docs(review): add review with findings for M5-006 |
| 3231e40 | chore(task): mark M5-006 as review |
| 6d29d25 | feat(rbac): implement CustomRolesEndpoints, PATCH role_id, and RoleResolver gate on POST /results |
| 4200736 | test(rbac): add failing integration tests for CustomRolesEndpoints |
| 95b2fd1 | feat(rbac): implement CustomRolesService, DTOs, and error types |
| c3f0002 | test(rbac): add failing tests for CustomRolesService business rules |
| 6f2becb | feat(rbac): implement RoleResolver for effective permission computation |
| f3ad327 | test(rbac): add failing tests for RoleResolver effective permission computation |
| 1c0a17f | feat(rbac): add CustomRole entity, RoleId helper, and EF migration AddCustomRoles |
| 5063c99 | test(rbac): add failing tests for RoleId wire-format helper |
| d93f1ec | feat(rbac): implement Permissions catalogue with built-in role sets |
| e4188e5 | test(rbac): add failing tests for Permissions catalogue |

## Files Changed

| File | Action |
|------|--------|
| `src/ApiTool.Backend/Rbac/Permissions.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/RoleId.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/RoleResolver.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRolesService.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRoleDto.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRoleError.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/CreateRoleRequest.cs` | created |
| `src/ApiTool.Backend/Rbac/CustomRoles/CustomRolesEndpoints.cs` | created |
| `src/ApiTool.Backend/Data/Entities/CustomRole.cs` | created |
| `src/ApiTool.Backend/Data/Entities/OrganizationMember.cs` | modified (added RoleId) |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modified (DbSet + mapping) |
| `src/ApiTool.Backend/Migrations/20260419073115_AddCustomRoles.cs` | created |
| `src/ApiTool.Backend/Migrations/20260419073115_AddCustomRoles.Designer.cs` | created |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modified |
| `src/ApiTool.Backend/Organizations/UpdateMemberRequest.cs` | modified (added RoleId) |
| `src/ApiTool.Backend/Organizations/MembersService.cs` | modified (UpdateMemberAsync with roleId) |
| `src/ApiTool.Backend/Organizations/MembersEndpoints.cs` | modified (plumb role_id) |
| `src/ApiTool.Backend/Invitations/InvitationsService.cs` | modified (RoleResolver gate) |
| `src/ApiTool.Backend/Results/ResultsEndpoints.cs` | modified (results.upload gate) |
| `src/ApiTool.Backend/Program.cs` | modified (DI + route registration) |
| `src/ApiTool.Backend.Tests/Rbac/PermissionsTests.cs` | created |
| `src/ApiTool.Backend.Tests/Rbac/RoleIdTests.cs` | created |
| `src/ApiTool.Backend.Tests/Rbac/RoleResolverTests.cs` | created |
| `src/ApiTool.Backend.Tests/Rbac/CustomRolesServiceTests.cs` | created |
| `src/ApiTool.Backend.Tests/Rbac/CustomRolesEndpointsTests.cs` | created |
| `src/ApiTool.Backend.Tests/Invitations/InvitationsServiceTests.cs` | modified |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
