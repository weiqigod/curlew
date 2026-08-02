namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Stable error codes for refresh-token failures.
/// Refs docs/SPECIFICATION.md:8243-8267, docs/api-errors.md.
/// </summary>
public static class RefreshSentinelErrors
{
    /// <summary>Refresh token was already rotated; entire family revoked.</summary>
    public const string Reused = "AUTH_REFRESH_REUSED";

    /// <summary>Refresh token is past its absolute 365-day deadline.</summary>
    public const string Expired = "AUTH_REFRESH_EXPIRED";

    /// <summary>Presented device_id does not match the token's bound device_id.</summary>
    public const string DeviceMismatch = "AUTH_DEVICE_MISMATCH";

    /// <summary>Refresh token is invalid, revoked, or not recognised.</summary>
    public const string Invalid = "AUTH_INVALID_REFRESH";
}
