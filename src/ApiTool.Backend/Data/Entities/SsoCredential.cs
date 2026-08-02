namespace ApiTool.Backend.Data.Entities;

/// <summary>Stores an IdP's public signing certificate for SAML assertion validation.</summary>
public sealed class SsoCredential
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>The organisation this credential belongs to.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Key identifier — SHA-256 of the certificate bytes (hex), used for cert rotation.</summary>
    public string Kid { get; set; } = string.Empty;

    /// <summary>PEM-encoded X.509 certificate for the IdP's signing key.</summary>
    public string PublicCertPem { get; set; } = string.Empty;

    /// <summary>UTC timestamp when this credential row was created.</summary>
    public DateTime CreatedAt { get; set; }
}
