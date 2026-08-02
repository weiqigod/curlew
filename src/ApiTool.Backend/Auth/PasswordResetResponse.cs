namespace ApiTool.Backend.Auth;

/// <summary>Response body for password-reset endpoints.</summary>
public sealed record PasswordResetResponse(bool Ok, string Message);
