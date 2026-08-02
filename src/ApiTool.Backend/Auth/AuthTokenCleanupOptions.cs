namespace ApiTool.Backend.Auth;

/// <summary>
/// Configuration options for <see cref="AuthTokenCleanupService"/>.
/// Bound from the <c>ApiTool:AuthTokenCleanup</c> configuration section.
/// </summary>
public sealed class AuthTokenCleanupOptions
{
    /// <summary>
    /// Configuration section key.
    /// </summary>
    public const string Section = "ApiTool:AuthTokenCleanup";

    /// <summary>
    /// Number of days to retain consumed, revoked, or expired tokens before deletion.
    /// Defaults to 7 days.
    /// </summary>
    public int RetentionDays { get; set; } = 7;
}
