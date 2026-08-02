namespace ApiTool.Backend.Auth;

/// <summary>Request body for <c>POST /api/v1/auth/password-reset/request</c>.</summary>
public sealed record PasswordResetRequest(string? Email);
