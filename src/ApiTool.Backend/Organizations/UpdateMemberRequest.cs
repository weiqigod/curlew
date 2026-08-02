namespace ApiTool.Backend.Organizations;

/// <summary>Request body for PATCH /api/v1/organizations/{id}/members/{memberId}.</summary>
/// <param name="Role">New built-in role: <c>admin</c> or <c>member</c>. Optional when <paramref name="RoleId"/> is set.</param>
/// <param name="RoleId">Optional custom role id (wire format: <c>role_&lt;32-hex&gt;</c>). When set, assigns the member to the custom role.</param>
public sealed record UpdateMemberRequest(string? Role = null, string? RoleId = null);
