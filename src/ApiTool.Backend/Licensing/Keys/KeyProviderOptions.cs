namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Options bound from <c>ApiTool:KeyProvider:*</c> configuration section.
/// </summary>
public sealed class KeyProviderOptions
{
    /// <summary>Configuration section path.</summary>
    public const string Section = "ApiTool:KeyProvider";

    /// <summary>Key provider mode. One of: <c>file</c>, <c>kms</c>. Default: <c>file</c>.</summary>
    public string Mode { get; set; } = "file";

    /// <summary>
    /// Environment label used as the first segment of the generated kid,
    /// e.g. <c>dev</c>, <c>staging</c>, <c>prod</c>. Default: <c>dev</c>.
    /// </summary>
    public string Env { get; set; } = "dev";

    /// <summary>File-provider–specific settings.</summary>
    public FileProviderOptions File { get; set; } = new();

    /// <summary>KMS-provider–specific settings.</summary>
    public KmsProviderOptions Kms { get; set; } = new();

    /// <summary>Options for the <c>file</c> mode key provider.</summary>
    public sealed class FileProviderOptions
    {
        /// <summary>
        /// Directory path holding <c>&lt;kid&gt;.pem</c> private-key files.
        /// Permissions are enforced to mode 0600 on Unix at write time.
        /// Default: <c>./Keys/signing</c>.
        /// </summary>
        public string Dir { get; set; } = "./Keys/signing";
    }

    /// <summary>Options for the <c>kms</c> mode key provider.</summary>
    public sealed class KmsProviderOptions
    {
        /// <summary>Google Cloud project ID.</summary>
        public string ProjectId { get; set; } = string.Empty;

        /// <summary>Google Cloud KMS location, e.g. <c>global</c> or <c>us-east1</c>.</summary>
        public string LocationId { get; set; } = string.Empty;

        /// <summary>Google Cloud KMS key ring name.</summary>
        public string KeyRing { get; set; } = string.Empty;
    }
}
