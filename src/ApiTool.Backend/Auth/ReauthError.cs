namespace ApiTool.Backend.Auth;

/// <summary>
/// Result codes returned by <see cref="IDeletionReauthService"/> operations.
/// </summary>
public enum ReauthError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>The supplied password did not match the user's stored hash.</summary>
    WrongPassword,

    /// <summary>The token could not be found (wrong hash or wrong user).</summary>
    TokenInvalid,

    /// <summary>The token was found but its <c>ExpiresAt</c> is in the past.</summary>
    TokenExpired,

    /// <summary>The token was found but was already consumed (single-use enforcement).</summary>
    TokenConsumed,
}
