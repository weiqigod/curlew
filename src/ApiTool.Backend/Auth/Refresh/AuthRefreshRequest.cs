namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Request body for <c>POST /api/v1/auth/refresh</c>.
/// Refs docs/SPECIFICATION.md:8228.
/// </summary>
public sealed record AuthRefreshRequest(
    string? RefreshToken,
    Guid? DeviceId);
