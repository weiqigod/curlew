namespace ApiTool.Backend.Auth;

/// <summary>Request body for <c>POST /api/v1/auth/email-verification/confirm</c>.</summary>
public sealed record EmailVerificationConfirmRequest(string? Token);
