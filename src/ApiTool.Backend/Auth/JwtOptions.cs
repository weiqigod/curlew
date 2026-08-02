namespace ApiTool.Backend.Auth;

/// <summary>Configuration options for JWT bearer token validation.</summary>
public sealed class JwtOptions
{
    /// <summary>The section name in appsettings.json.</summary>
    public const string Section = "Jwt";

    /// <summary>Symmetric signing key. Must be at least 32 bytes (256-bit).</summary>
    public string SigningKey { get; set; } = string.Empty;

    /// <summary>Expected token issuer (<c>iss</c> claim).</summary>
    public string Issuer { get; set; } = string.Empty;

    /// <summary>Expected token audience (<c>aud</c> claim).</summary>
    public string Audience { get; set; } = string.Empty;
}
