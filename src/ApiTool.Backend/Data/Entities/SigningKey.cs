namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// One row per registered JWT signing key (kid). Used by IKeyProvider
/// and exposed via /api/v1/.well-known/jwks.json.
/// Refs docs/SPECIFICATION.md:9715-9735.
/// </summary>
public sealed class SigningKey
{
    /// <summary>Key identifier — format: <c>&lt;env&gt;-es256-&lt;YYYYMM&gt;-&lt;6hex&gt;</c>.</summary>
    public string Kid { get; set; } = string.Empty;

    /// <summary>Signing algorithm identifier, e.g. <c>ES256</c>.</summary>
    public string Algorithm { get; set; } = "ES256";

    /// <summary>Lifecycle state — one of <c>current</c>, <c>next</c>, <c>verifying</c>, <c>revoked</c>.</summary>
    public string Status { get; set; } = "current";

    /// <summary>KMS resource name when key material is held by Google KMS; null for file-based keys.</summary>
    public string? KmsKeyId { get; set; }

    /// <summary>Public key serialised as a JWK JSON object (TEXT on SQLite; jsonb on Postgres).</summary>
    public string PublicKeyJwkJson { get; set; } = string.Empty;

    /// <summary>UTC timestamp when this row was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp when this key was promoted to <c>current</c>.</summary>
    public DateTime? PromotedAt { get; set; }

    /// <summary>UTC timestamp at which this key begins its retirement window.</summary>
    public DateTime? RetiringAt { get; set; }

    /// <summary>UTC timestamp when this key was revoked.</summary>
    public DateTime? RevokedAt { get; set; }

    /// <summary>Human-readable reason for revocation, e.g. <c>compromise</c>.</summary>
    public string? RevokeReason { get; set; }
}
