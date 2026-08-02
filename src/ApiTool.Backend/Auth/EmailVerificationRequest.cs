namespace ApiTool.Backend.Auth;

/// <summary>Request body for <c>POST /api/v1/auth/email-verification/resend</c>.</summary>
public sealed record EmailVerificationRequest(string? Email);
