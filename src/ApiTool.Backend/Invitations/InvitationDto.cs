namespace ApiTool.Backend.Invitations;

/// <summary>Wire model for an invitation resource.</summary>
/// <param name="Id">Wire-format invitation id (<c>inv_&lt;hex&gt;</c>).</param>
/// <param name="OrgId">Wire-format organization id (<c>org_&lt;hex&gt;</c>).</param>
/// <param name="Email">Email address of the invitee.</param>
/// <param name="Role">Role the invitee will receive on acceptance.</param>
/// <param name="ExpiresAt">UTC expiry time.</param>
/// <param name="CreatedAt">UTC creation time.</param>
/// <param name="AcceptedAt">UTC acceptance time, or null if pending.</param>
/// <param name="RevokedAt">UTC revocation time, or null if not revoked.</param>
public sealed record InvitationDto(
    string Id,
    string OrgId,
    string Email,
    string Role,
    DateTime ExpiresAt,
    DateTime CreatedAt,
    DateTime? AcceptedAt,
    DateTime? RevokedAt);
