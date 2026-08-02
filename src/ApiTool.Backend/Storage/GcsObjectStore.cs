using Google.Apis.Auth.OAuth2;
using Google.Cloud.Storage.V1;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Storage;

/// <summary>
/// <see cref="IObjectStore"/> implementation backed by Google Cloud Storage.
/// Used in the SaaS profile; configured via <see cref="ObjectStoreOptions.GcsOptions"/>.
/// Signed URLs are generated using V4 signing without requiring a live GCS call — only the
/// service-account credentials (JSON key file) are needed, making unit-level signing tests
/// possible without real cloud spend.
/// </summary>
public sealed class GcsObjectStore : IObjectStore, IAsyncDisposable
{
    private readonly StorageClient _client;
    private readonly UrlSigner _urlSigner;
    private readonly string _bucket;
    private readonly ILogger<GcsObjectStore> _logger;

    /// <summary>Initialises from options; loads service-account credentials from disk.</summary>
    public GcsObjectStore(IOptions<ObjectStoreOptions> options, ILogger<GcsObjectStore> logger)
    {
        ArgumentNullException.ThrowIfNull(options);
        var gcs = options.Value.Gcs;
        _bucket = gcs.Bucket;
        _logger = logger;

        GoogleCredential credential;
        ServiceAccountCredential serviceAccountCredential;
        if (!string.IsNullOrEmpty(gcs.ServiceAccountKeyPath))
        {
            using var stream = File.OpenRead(gcs.ServiceAccountKeyPath);
            // Use the non-deprecated CredentialFactory path.
            var jsonCredential = Google.Apis.Auth.OAuth2.ServiceAccountCredential
                .FromServiceAccountData(stream);
            serviceAccountCredential = jsonCredential;
            credential = jsonCredential.ToGoogleCredential();
        }
        else
        {
            credential = GoogleCredential.GetApplicationDefault();
            serviceAccountCredential = credential.UnderlyingCredential as ServiceAccountCredential
                ?? throw new InvalidOperationException(
                    "GcsObjectStore requires a service-account credential for URL signing. " +
                    "Provide a service-account JSON key via ApiTool:ObjectStore:Gcs:ServiceAccountKeyPath.");
        }

        _client = StorageClient.Create(credential);

        // UrlSigner for V4 signed URLs — operates offline against the credential; no GCS call.
        _urlSigner = UrlSigner.FromCredential(serviceAccountCredential);
    }

    /// <summary>Initialises from explicit clients (used in tests to inject a fake signer).</summary>
    internal GcsObjectStore(StorageClient client, UrlSigner urlSigner, string bucket, ILogger<GcsObjectStore> logger)
    {
        _client = client;
        _urlSigner = urlSigner;
        _bucket = bucket;
        _logger = logger;
    }

    /// <inheritdoc/>
    public async Task PutAsync(string key, Stream content, string contentType, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);
        ArgumentNullException.ThrowIfNull(content);

        await _client.UploadObjectAsync(
            _bucket, key, contentType, content,
            cancellationToken: ct);

        _logger.LogDebug("GcsObjectStore: uploaded {Bucket}/{Key}", _bucket, key);
    }

    /// <inheritdoc/>
    public async Task<Uri> GetSignedUrlAsync(string key, TimeSpan ttl, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        // V4 signed URL — signing is an offline operation; no GCS API call is made.
        var url = await _urlSigner.SignAsync(
            _bucket, key,
            ttl,
            HttpMethod.Get,
            signingVersion: SigningVersion.V4);

        return new Uri(url);
    }

    /// <inheritdoc/>
    public async Task<Stream> GetAsync(string key, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        var ms = new MemoryStream();
        try
        {
            await _client.DownloadObjectAsync(_bucket, key, ms, cancellationToken: ct);
        }
        catch (Google.GoogleApiException ex) when (ex.HttpStatusCode == System.Net.HttpStatusCode.NotFound)
        {
            throw new KeyNotFoundException($"Object key '{key}' not found in GCS bucket '{_bucket}'.", ex);
        }

        ms.Seek(0, SeekOrigin.Begin);
        return ms;
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync()
    {
        _client.Dispose();
        return ValueTask.CompletedTask;
    }
}
