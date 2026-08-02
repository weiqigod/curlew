namespace ApiTool.Backend.Auth;

/// <summary>Result of email-verification flow operations.</summary>
public enum EmailVerificationError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>Token is unknown, expired, consumed, or revoked.</summary>
    TokenInvalid,
}
