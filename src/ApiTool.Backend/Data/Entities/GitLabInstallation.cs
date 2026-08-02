// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration), :10893-10920 (schema).
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Per-project GitLab integration with PAT-based auth.
/// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration), :10893-10920 (schema).
/// </summary>
[GdprTable(GdprTableKind.ExcludedFromBoth)]
public sealed class GitLabInstallation
{
    /// <summary>Surrogate primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization. FK organizations(id).</summary>
    public Guid OrgId { get; set; }

    /// <summary>GitLab's numeric project ID (per-instance namespace).</summary>
    public long ProjectId { get; set; }

    /// <summary>Display path, e.g. "gitlab.com/group/project".</summary>
    public string ProjectPath { get; set; } = string.Empty;

    /// <summary>Default "https://gitlab.com"; set to a self-managed URL when applicable.</summary>
    public string GitLabBaseUrl { get; set; } = "https://gitlab.com";

    /// <summary>PEM CA bundle for self-managed instances using a private CA. Nullable.</summary>
    public string? GitLabCaBundle { get; set; }

    /// <summary>AES-256-GCM ciphertext of the PAT (envelope per IGitLabKeyProvider).</summary>
    public byte[] AccessTokenCiphertext { get; set; } = Array.Empty<byte>();

    /// <summary>Wrapping-key id ("gitlab-kek-file-v1" or full KMS resource path).</summary>
    public string AccessTokenKid { get; set; } = string.Empty;

    /// <summary>Set when the customer revokes the PAT (detected via 401 from GitLab).</summary>
    public DateTime? AccessTokenRevokedAt { get; set; }

    /// <summary>Inbound-webhook secret ciphertext (same envelope shape). Nullable until M16-015.</summary>
    public byte[]? WebhookSecretCiphertext { get; set; }

    /// <summary>Last rotation moment for the webhook secret. Nullable.</summary>
    public DateTime? WebhookSecretRotatedAt { get; set; }

    /// <summary>Insert moment (UTC).</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>Last mutation moment (UTC). Updated on PAT rotation, base-URL change, etc.</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>Soft-delete marker. Row is preserved for audit; PAT ciphertext purged on hard-delete (30-day grace, M16-016 scope).</summary>
    public DateTime? DeletedAt { get; set; }
}
