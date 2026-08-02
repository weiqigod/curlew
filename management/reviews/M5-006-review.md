# Code Review: M5-006

**Task:** Backend: custom roles and granular permissions
**Reviewer:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-006-custom-roles-permissions
**Iteration:** 3 (post-improvement 2)

## Verdict: PASS

## Findings

No findings. All previous findings have been resolved.

## Resolved Findings from Prior Iterations

**Iteration 1 (6 findings — all resolved):**
- `BillingView` removed from `BuiltInMember` per spec matrix ✓
- `SeatsAdd` removed from `BuiltInAdmin` per spec matrix ✓
- Test 14 invite-gate assertion added ✓
- `InvitationsService` uses `RoleResolver.HasPermissionAsync` for `members.invite` ✓
- `Permissions.cs` comment updated to accurately describe member permissions ✓
- Test 14 method name accurate ✓

**Iteration 2 (3 findings — all resolved):**
- `UpdateMemberRoleAsync` dead code fully removed from `MembersService.cs` ✓
- Misleading comment in `MembersEndpoints.cs` line 129 corrected to "If neither role nor role_id is provided, return 400." ✓
- `Admin_does_not_have_seats_remove_per_spec_matrix` test added to `PermissionsTests.cs` ✓

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return typed enum errors via tuple returns; no swallowed exceptions; exception handling in `ToDto` and `DeserializeKeys` returns safe defaults (empty list) rather than propagating. |
| Input Validation | PASS | `CreateRoleRequest` validated: name trimmed, empty/reserved/too-long name rejected, unknown permission keys rejected with offending key in body. `RoleId` and `OrgId` TryParse before use. `null` JSON body handled by ASP.NET model binding. |
| Naming | PASS | No stuttering. All exported types, methods, and properties have doc comments. `RoleResolver`, `CustomRolesService`, `CustomRoleDto`, `RoleId`, `CustomRoleError` follow existing codebase conventions. |
| Code Organization | PASS | `Rbac/CustomRoles/` folder clean, single responsibility per file. `RoleResolver` and `CustomRolesService` DI-registered in `Program.cs`. No circular dependencies. EF model configuration correct (unique constraint on OrgId+Name, FK on CreatedBy with Restrict on delete). |
| Correctness | PASS | Spec matrix permissions are correct. `RoleResolver` correctly computes effective permission set: custom role overrides built-in, falls back to built-in for deleted or cross-org role references, unions with direct overrides. Invite endpoint permission-catalogue-gated via `RoleResolver`. Results upload endpoint gated per custom role. |
| Test Quality | PASS | All 8 task behaviors covered by dedicated tests. Dead code removed. No misleading comments. Table-driven tests used where appropriate. Integration tests use `BackendFactory` with real HTTP stack. Unit tests use SQLite in-memory via `TestDb`. |

## Test Coverage

- Observable filter `FullyQualifiedName~Rbac&FullyQualifiedName~CustomRoles`: **37 tests, 0 failures** (threshold ≥12 met)
- Full suite: **484 tests, 0 failures**
- CustomRolesEndpointsTests: 15 tests covering all 8 behaviors plus edge cases
- CustomRolesServiceTests: 12 unit tests (create, list, delete with permission, audit)
- RoleResolverTests: 9 unit tests (built-in roles, custom role override, fallback, cross-org isolation, HasPermission)
- PermissionsTests: 10 tests (catalogue completeness, set membership per spec matrix, all sets subsets of All)
- RoleIdTests: 5 tests (round-trip, format, TryParse rejects invalid including null/empty/wrong-prefix/bad-hex)
- InvitationsServiceTests: 4 tests (now exercises `RoleResolver` injection)

## Summary

The implementation is complete and correct across all three iterations. All spec matrix permissions are accurate, the `RoleResolver` correctly computes effective permissions including custom-role overrides, invitation and result-upload endpoints are properly gated through the resolver, dead code has been removed, and all 8 task behaviors are covered by focused tests. No findings remain.
