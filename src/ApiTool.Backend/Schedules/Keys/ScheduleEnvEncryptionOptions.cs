// Configuration options for schedule env_vars encryption (M18-009).
namespace ApiTool.Backend.Schedules.Keys;

/// <summary>
/// Top-level configuration for schedule env_vars envelope encryption.
/// Bound from <c>ApiTool:Schedules:Encryption</c>
/// (env vars use <c>SCHEDULES__</c> prefix alias — see Program.cs).
/// </summary>
public sealed class ScheduleEnvEncryptionOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:Schedules:Encryption";

    /// <summary>Key provider configuration (file or KMS mode).</summary>
    public KeyProviderConfig KeyProvider { get; set; } = new();

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
