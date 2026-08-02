namespace ApiTool.Backend.Auth;

/// <summary>Request body for <c>POST /api/v1/auth/password-reset/confirm</c>.</summary>
public sealed record PasswordResetConfirmRequest(string? Token, string? NewPassword);
