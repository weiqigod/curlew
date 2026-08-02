namespace ApiTool.Backend.Storage;

/// <summary>
/// Options bound to <c>ApiTool:ObjectStore</c> (env-var prefix <c>OBJECTSTORE__</c>).
/// Select the implementation via <see cref="Provider"/>: <c>in_memory</c> (test only),
/// <c>s3</c> (default — also used with MinIO), or <c>gcs</c> (SaaS profile).
/// </summary>
public sealed class ObjectStoreOptions
{
    /// <summary>Config section key.</summary>
    public const string Section = "ApiTool:ObjectStore";

    /// <summary><c>in_memory</c>, <c>s3</c>, or <c>gcs</c>. Defaults to <c>in_memory</c> in Testing.</summary>
    public string Provider { get; set; } = "in_memory";

    /// <summary>S3-compatible options (used when <see cref="Provider"/> is <c>s3</c>).</summary>
    public S3Options S3 { get; set; } = new();

    /// <summary>GCS options (used when <see cref="Provider"/> is <c>gcs</c>).</summary>
    public GcsOptions Gcs { get; set; } = new();

    /// <summary>S3-compatible store options.</summary>
    public sealed class S3Options
    {
        /// <summary>Optional custom endpoint URL (for MinIO: <c>http://minio:9000</c>).</summary>
        public string? Endpoint { get; set; }

        /// <summary>AWS region (default: <c>us-east-1</c>). Ignored by MinIO but required by the SDK.</summary>
        public string Region { get; set; } = "us-east-1";

        /// <summary>Bucket name.</summary>
        public string Bucket { get; set; } = "apitool-exports";

        /// <summary>Access key (env: <c>OBJECTSTORE__S3__ACCESSKEY</c>).</summary>
        public string? AccessKey { get; set; }

        /// <summary>Secret key (env: <c>OBJECTSTORE__S3__SECRETKEY</c>).</summary>
        public string? SecretKey { get; set; }

        /// <summary>Enable path-style URLs (required for MinIO, default: false).</summary>
        public bool UsePathStyle { get; set; }
    }

    /// <summary>Google Cloud Storage options.</summary>
    public sealed class GcsOptions
    {
        /// <summary>GCS bucket name.</summary>
        public string Bucket { get; set; } = "apitool-exports";

        /// <summary>Path to the service-account JSON key file.</summary>
        public string? ServiceAccountKeyPath { get; set; }
    }
}
