# Improvement Report: M5-006

**Task:** Backend: custom roles and granular permissions
**Date:** 2026-04-19
**Review:** management/reviews/M5-006-review.md

## Resolved Findings — Iteration 1

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `BillingView` included in `BuiltInMember` — spec matrix says billing.view is ✗ for Member | Removed `BillingView` from `BuiltInMember` set in `Permissions.cs`. Added red test `Member_does_not_have_billing_view_per_spec_matrix` | ✓ tests pass |
| 2 | Critical | `SeatsAdd` included in `BuiltInAdmin` — spec matrix says seats.add is ✓ Owner only | Removed `SeatsAdd` from `BuiltInAdmin` set in `Permissions.cs`. Added red test `Admin_does_not_have_seats_add_per_spec_matrix` | ✓ tests pass |
| 3 | High | Test 14 only asserts the upload success path; the "fails invite gate" branch was never asserted | Added assertion: custom-role member without `members.invite` calls `POST /organizations/{id}/invitations` and receives `403 permission_denied` | ✓ tests pass |
| 4 | High | Invite endpoint gates on `OrgRole.Member` (built-in role), not `RoleResolver`. Custom role with `members.invite` would still be blocked; custom role without it would be blocked only coincidentally | Changed `InvitationsService.CreateAsync` to inject `RoleResolver` and call `HasPermissionAsync(members.invite)` instead of the raw `OrgRole.Member` check. Added test 14b: custom-role member WITH `members.invite` can invite (201) | ✓ tests pass |
| 5 | Medium | Comment on `BuiltInMember` says "read-only access" but member has `ResultsUpload` | Updated comment to "standard contributor access — can view and upload results, view dashboard, and view org data" | ✓ code accurate |
| 6 | Low | Test method name `fails_members_invite_gate` did not match test body which never exercised invite endpoint | Test name is now accurate: both the upload-passes and invite-fails branches are covered in the same test method | ✓ tests pass |

## Resolved Findings — Iteration 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `UpdateMemberRoleAsync` is dead code — defined in `MembersService` but never called from any production code path | Removed the method entirely from `MembersService.cs`. No callers exist in production or test code. | ✓ build clean, 484 tests pass |
| 2 | Low | Comment in `MembersEndpoints.cs` line 129 says "the existing `UpdateMemberRoleAsync` path applies" but code actually falls through to return `400 bad_request` | Updated comment to: "If neither role nor role_id is provided, return 400." | ✓ code accurate |
| 3 | Low | `seats.remove` is Owner-only per spec matrix but had no test asserting Admin and Member do not have it | Added `Admin_does_not_have_seats_remove_per_spec_matrix` fact in `PermissionsTests.cs` | ✓ test passes (484 total) |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/...` | PASS |
| Coverage (line-rate) | 92.0% |
| `golangci-lint run` | N/A — C# project, no Go linter applicable |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6c37881 | fix(rbac): correct BuiltInMember and BuiltInAdmin permission sets per spec | Iter1 #1, #2, #5 |
| 9da5d76 | fix(rbac): gate invitations on RoleResolver permission check (members.invite) | Iter1 #3, #4, #6 |
| fc7a59f | fix(rbac): remove dead code, fix comment, add seats.remove test | Iter2 #1, #2, #3 |

## Summary

Iteration 1: 6/6 findings resolved. 0 deferred.
Iteration 2: 3/3 findings resolved. 0 deferred.

Key changes (iteration 2):
- `src/ApiTool.Backend/Organizations/MembersService.cs` — removed dead `UpdateMemberRoleAsync` method
- `src/ApiTool.Backend/Organizations/MembersEndpoints.cs` — corrected misleading comment in `UpdateMember` handler
- `src/ApiTool.Backend.Tests/Rbac/PermissionsTests.cs` — added `Admin_does_not_have_seats_remove_per_spec_matrix` test

Test count: 484 tests, all passing (up from 483 after iteration 1).
