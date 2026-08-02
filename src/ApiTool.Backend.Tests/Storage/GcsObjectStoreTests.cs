using ApiTool.Backend.Storage;
using Google.Apis.Auth.OAuth2;
using Google.Cloud.Storage.V1;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Storage;

/// <summary>
/// Unit tests for <see cref="GcsObjectStore"/> URL-signing logic.
/// Uses GCS's <see cref="UrlSigner"/> in offline mode (no live GCS calls).
/// Requires a fixture service-account JSON key; tests are skipped when
/// the <c>GCS_SERVICE_ACCOUNT_KEY_PATH</c> environment variable is not set.
/// </summary>
public class GcsObjectStoreTests
{
    private static bool GcsKeyAvailable =>
        !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("GCS_SERVICE_ACCOUNT_KEY_PATH"))
        && File.Exists(Environment.GetEnvironmentVariable("GCS_SERVICE_ACCOUNT_KEY_PATH")!);

    [Fact]
    public void Options_bind_gcs_provider_correctly()
    {
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "gcs",
            Gcs = new ObjectStoreOptions.GcsOptions
            {
                Bucket = "apitool-exports",
                ServiceAccountKeyPath = null, // will use ADC — but we only test option binding here
            },
        });

        // Just verify option binding; construction would fail without credentials so we only
        // assert the options are readable.
        opts.Value.Provider.Should().Be("gcs");
        opts.Value.Gcs.Bucket.Should().Be("apitool-exports");
    }

    [SkippableFact]
    [Trait("Category", "gcs-unit")]
    public async Task SignedUrl_contains_bucket_and_key()
    {
        Skip.IfNot(GcsKeyAvailable, "GCS_SERVICE_ACCOUNT_KEY_PATH not set — skipping GCS signing test");

        var keyPath = Environment.GetEnvironmentVariable("GCS_SERVICE_ACCOUNT_KEY_PATH")!;

        // Build an offline URL signer from the fixture service-account key.
        ServiceAccountCredential credential;
        using (var stream = File.OpenRead(keyPath))
            credential = ServiceAccountCredential.FromServiceAccountData(stream);

        var urlSigner = UrlSigner.FromCredential(credential);

        // Build a real GcsObjectStore using the internal ctor (no network calls for signing).
        using var clientFactory = StorageClient.Create(credential.ToGoogleCredential());
        var store = new GcsObjectStore(
            clientFactory,
            urlSigner,
            "test-bucket",
            NullLogger<GcsObjectStore>.Instance);

        var url = await store.GetSignedUrlAsync(
            "exports/user123/request456.json",
            TimeSpan.FromHours(24),
            default);

        url.Should().NotBeNull();
        url.AbsoluteUri.Should().Contain("test-bucket");
        url.AbsoluteUri.Should().Contain("exports");
        url.AbsoluteUri.Should().Contain("X-Goog-Signature");
    }

    [SkippableFact]
    [Trait("Category", "gcs-unit")]
    public async Task SignedUrl_ttl_reflected_in_X_Goog_Expires_param()
    {
        Skip.IfNot(GcsKeyAvailable, "GCS_SERVICE_ACCOUNT_KEY_PATH not set — skipping GCS signing test");

        var keyPath = Environment.GetEnvironmentVariable("GCS_SERVICE_ACCOUNT_KEY_PATH")!;

        ServiceAccountCredential credential;
        using (var stream = File.OpenRead(keyPath))
            credential = ServiceAccountCredential.FromServiceAccountData(stream);

        var urlSigner = UrlSigner.FromCredential(credential);
        using var clientFactory = StorageClient.Create(credential.ToGoogleCredential());
        var store = new GcsObjectStore(
            clientFactory,
            urlSigner,
            "test-bucket",
            NullLogger<GcsObjectStore>.Instance);

        // 24h = 86400 seconds
        var url = await store.GetSignedUrlAsync("some/key.json", TimeSpan.FromHours(24), default);

        // V4 signed URLs embed X-Goog-Expires (or equivalent) in the query.
        url.AbsoluteUri.Should().MatchRegex("X-Goog-Expires=8640[0-9]");
    }
}
