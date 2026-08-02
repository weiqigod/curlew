namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Configuration options for token issuance.
/// Binds to the <c>"Jwt"</c> section.
/// </summary>
public sealed class TokenIssuerOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "Jwt";

    /// <summary>The <c>iss</c> claim value emitted on all tokens.</summary>
    public string Issuer { get; set; } = "https://api.apitool.dev";

    /// <summary>The <c>aud</c> for License JWTs.</summary>
    public string LicenseAudience { get; set; } = "apitool-license";

    /// <summary>The <c>aud</c> for Access tokens.</summary>
    public string AccessAudience { get; set; } = "apitool-cli-api";

    /// <summary>License JWT validity window.</summary>
    public TimeSpan LicenseLifetime { get; set; } = TimeSpan.FromDays(30);

    /// <summary>Grace period appended to License JWT expiry for offline tolerance.</summary>
    public TimeSpan LicenseGrace { get; set; } = TimeSpan.FromDays(14);

    /// <summary>Access token validity window.</summary>
    public TimeSpan AccessLifetime { get; set; } = TimeSpan.FromHours(1);

    /// <summary>Clock skew tolerance used by token validators.</summary>
    public TimeSpan SkewTolerance { get; set; } = TimeSpan.FromSeconds(30);
}
