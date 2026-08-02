namespace ApiTool.Backend.GitHub;

/// <summary>
/// Options bound from <c>ApiTool:GitHubApp</c> (env: <c>GITHUB_APP__*</c>).
/// </summary>
public sealed class GitHubAppOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:GitHubApp";

    /// <summary>Numeric GitHub App ID (used as JWT iss).</summary>
    public long AppId { get; set; }

    /// <summary>Operator-chosen GitHub App slug (Open Decision #4 default, :8350).</summary>
    public string Slug { get; set; } = string.Empty;

    /// <summary>Key-provider mode: <c>file</c> | <c>kms</c>. Default: <c>file</c>.</summary>
    public KeyProviderConfig KeyProvider { get; set; } = new();

    /// <summary>
    /// 32+ char random secret used to sign the install-URL state token (HMAC-SHA256).
    /// Bound from <c>APITOOL__GITHUBAPP__STATESIGNINGKEY</c>.
    /// </summary>
    public string StateSigningKey { get; set; } = string.Empty;

    /// <summary>Key provider selection and sub-configuration.</summary>
    public sealed class KeyProviderConfig
    {
        /// <summary>Provider mode: <c>file</c> (self-hosted) or <c>kms</c> (SaaS).</summary>
        public string Mode { get; set; } = "file";

        /// <summary>Configuration for the file-based key provider.</summary>
        public FileConfig File { get; set; } = new();

        /// <summary>Configuration for the Google KMS-based key provider.</summary>
        public KmsConfig Kms { get; set; } = new();
    }

    /// <summary>Configuration for the file-based key provider.</summary>
    public sealed class FileConfig
    {
        /// <summary>Path to the PEM file (mode 0600 enforced on Unix at startup).</summary>
        public string Path { get; set; } = string.Empty;
    }

    /// <summary>Configuration for the Google KMS-based key provider.</summary>
    public sealed class KmsConfig
    {
        /// <summary>
        /// Fully-qualified KMS key version resource, e.g.
        /// <c>projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1</c>.
        /// Must be an <c>RSA_SIGN_PKCS1_2048_SHA256</c> key.
        /// </summary>
        public string KmsKeyId { get; set; } = string.Empty;
    }
}
