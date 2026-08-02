namespace ApiTool.Backend.Auth;

/// <summary>Response body for a successful <c>POST /api/v1/auth/login</c> request.</summary>
public sealed record LoginResponse(
    string AccessToken,
    string TokenType,
    int ExpiresIn,
    string Role);
