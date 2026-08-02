namespace ApiTool.Backend.Auth;

/// <summary>Response body for email-verification endpoints.</summary>
public sealed record EmailVerificationResponse(bool Ok, string Message);
