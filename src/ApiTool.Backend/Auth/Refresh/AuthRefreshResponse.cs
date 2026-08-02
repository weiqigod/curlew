namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Successful response body for <c>POST /api/v1/auth/refresh</c>.
/// Carries the unified mint contract: License JWT, Access token, and new Refresh token.
/// Refs docs/SPECIFICATION.md:8228.
/// </summary>
public sealed record AuthRefreshResponse(
    string LicenseJwt,
    string AccessToken,
    string RefreshToken);
