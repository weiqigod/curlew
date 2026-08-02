// Configuration options for GitLab integration (M16-013).
// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration).
namespace ApiTool.Backend.GitLab;

/// <summary>
/// Top-level configuration for the GitLab integration.
/// Bound from <c>ApiTool:GitLab</c> (env vars use <c>GITLAB__</c> prefix alias — see Program.cs).
/// </summary>
public sealed class GitLabOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:GitLab";

    /// <summary>Key provider configuration (file or KMS mode).</summary>
    public KeyProviderConfig KeyProvider { get; set; } = new();

    /// <summary>Poster configuration (M16-014).</summary>
    public PosterConfig Poster { get; set; } = new();

    /// <summary>Options for the outbound GitLab commit-status poster.</summary>
    public sealed class PosterConfig
    {
        /// <summary>
        /// When true, the poster permits <c>gitlab_base_url</c> values with the <c>http://</c> scheme.
        /// Default false (production-safe). Override via env <c>GITLAB__ALLOW_HTTP=true</c>
        /// for local development against a non-TLS stub server.
        /// Refs M16-014 task YAML Open Decision 2.
        /// </summary>
        public bool AllowHttp { get; set; } = false;
    }

    /// <summary>Key provider sub-options.</summary>
    public sealed class KeyProviderConfig
    {
        /// <summary>"file" (default) or "kms".</summary>
        public string Mode { get; set; } = "file";

        /// <summary>File-mode KEK options.</summary>
        public FileConfig File { get; set; } = new();

        /// <summary>KMS-mode options.</summary>
        public KmsConfig Kms { get; set; } = new();
    }

    /// <summary>Options for file-based KEK (self-hosted deployments).</summary>
    public sealed class FileConfig
    {
        /// <summary>Absolute path to the 32-byte AES-256 KEK binary. Must exist and be readable (mode 0600 recommended).</summary>
        public string KekPath { get; set; } = string.Empty;
    }

    /// <summary>Options for Google Cloud KMS KEK (SaaS deployments).</summary>
    public sealed class KmsConfig
    {
        /// <summary>Full KMS resource path, e.g. projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1.</summary>
        public string KmsKeyId { get; set; } = string.Empty;
    }
}
