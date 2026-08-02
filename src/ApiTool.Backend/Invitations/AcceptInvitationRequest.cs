namespace ApiTool.Backend.Invitations;

/// <summary>Request body for POST /api/v1/invitations/accept.</summary>
/// <param name="Token">The raw invitation token from the invite email.</param>
public sealed record AcceptInvitationRequest(string Token);
