namespace ApiTool.Backend.Organizations;

/// <summary>Wire model for an organization member.</summary>
/// <param name="UserId">The member's user id (raw Guid string).</param>
/// <param name="Role">The member's built-in role string (owner | admin | member).</param>
/// <param name="JoinedAt">UTC timestamp when the user joined.</param>
/// <param name="RoleId">
/// Wire-format custom-role id (e.g. <c>role_abc…</c>) when the member has been
/// assigned a custom role, otherwise <see langword="null"/>. The frontend's
/// per-role member counter relies on this being populated to distinguish
/// custom-role members from built-in-role members.
/// </param>
public sealed record MemberDto(string UserId, string Role, DateTime JoinedAt, string? RoleId = null);
