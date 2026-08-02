namespace ApiTool.Backend.Auth;

/// <summary>
/// Result of password-reset flow operations. <c>None</c> = success.
/// </summary>
public enum PasswordResetError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>Token is unknown, expired, consumed, or revoked.</summary>
    TokenInvalid,

    /// <summary>The candidate password scored below the minimum strength.</summary>
    PasswordTooWeak,
}
