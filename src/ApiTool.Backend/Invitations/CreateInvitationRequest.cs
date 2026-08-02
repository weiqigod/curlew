namespace ApiTool.Backend.Invitations;

/// <summary>Request body for POST /api/v1/organizations/{id}/invitations.</summary>
/// <param name="Email">Email address to invite.</param>
/// <param name="Role">Role to assign on acceptance: <c>admin</c> or <c>member</c>.</param>
public sealed record CreateInvitationRequest(string Email, string Role);
