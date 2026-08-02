using Amazon.S3;
using Amazon.S3.Model;
using ApiTool.Backend.Storage;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Storage;

/// <summary>
/// Unit and integration tests for <see cref="S3ObjectStore"/>.
/// MinIO-backed tests are tagged <c>Category=minio-integration</c> and require a running
/// MinIO sidecar (available in the docker-compose.test.yml stack).  They are skipped
/// automatically unless the <c>MINIO_ENDPOINT</c> environment variable is set.
/// </summary>
public class S3ObjectStoreTests
{
    // ── Unit-level tests via options (no MinIO, will fail to connect — verify options binding) ─

    [Fact]
    public async Task Options_bind_s3_provider_correctly()
    {
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "s3",
            S3 = new ObjectStoreOptions.S3Options
            {
                Endpoint = "http://minio:9000",
                Bucket = "apitool-exports",
                AccessKey = "minioadmin",
                SecretKey = "minioadmin",
                UsePathStyle = true,
                Region = "us-east-1",
            },
        });

        // Construction succeeds — client is lazy; no network I/O in ctor.
        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);
        store.Should().NotBeNull();
    }

    [Fact]
    public async Task Options_bind_default_bucket()
    {
        var opts = Options.Create(new ObjectStoreOptions { Provider = "s3" });
        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);
        store.Should().NotBeNull();
    }

    // ── MinIO integration tests (require MINIO_ENDPOINT env var) ──────────

    private static bool MinioAvailable =>
        !string.IsNullOrEmpty(Environment.GetEnvironmentVariable("MINIO_ENDPOINT"));

    [SkippableFact]
    [Trait("Category", "minio-integration")]
    public async Task MinIO_put_then_get_round_trip()
    {
        Skip.IfNot(MinioAvailable, "MINIO_ENDPOINT not set — skipping MinIO integration test");

        var endpoint = Environment.GetEnvironmentVariable("MINIO_ENDPOINT")!;
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "s3",
            S3 = new ObjectStoreOptions.S3Options
            {
                Endpoint = endpoint,
                Bucket = "apitool-test-exports",
                AccessKey = "minioadmin",
                SecretKey = "minioadmin",
                UsePathStyle = true,
                Region = "us-east-1",
            },
        });

        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);

        var key = $"integration-test/{Guid.NewGuid():N}.json";
        var payload = """{"test":true}"""u8.ToArray();
        await store.PutAsync(key, new MemoryStream(payload), "application/json", default);

        await using var stream = await store.GetAsync(key, default);
        using var reader = new StreamReader(stream);
        var body = await reader.ReadToEndAsync();
        body.Should().Contain("true");
    }

    [SkippableFact]
    [Trait("Category", "minio-integration")]
    public async Task MinIO_signed_url_contains_key_segment()
    {
        Skip.IfNot(MinioAvailable, "MINIO_ENDPOINT not set — skipping MinIO integration test");

        var endpoint = Environment.GetEnvironmentVariable("MINIO_ENDPOINT")!;
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "s3",
            S3 = new ObjectStoreOptions.S3Options
            {
                Endpoint = endpoint,
                Bucket = "apitool-test-exports",
                AccessKey = "minioadmin",
                SecretKey = "minioadmin",
                UsePathStyle = true,
                Region = "us-east-1",
            },
        });

        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);

        var key = $"integration-test/{Guid.NewGuid():N}.json";
        await store.PutAsync(key, new MemoryStream("""{"ok":1}"""u8.ToArray()), "application/json", default);

        var url = await store.GetSignedUrlAsync(key, TimeSpan.FromHours(1), default);
        url.Scheme.Should().Be(new Uri(endpoint).Scheme);
        url.AbsoluteUri.Should().Contain(Uri.EscapeDataString(key).Replace("%2F", "/"));
    }

    [SkippableFact]
    [Trait("Category", "minio-integration")]
    public async Task MinIO_get_missing_key_throws_KeyNotFoundException()
    {
        Skip.IfNot(MinioAvailable, "MINIO_ENDPOINT not set — skipping MinIO integration test");

        var endpoint = Environment.GetEnvironmentVariable("MINIO_ENDPOINT")!;
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "s3",
            S3 = new ObjectStoreOptions.S3Options
            {
                Endpoint = endpoint,
                Bucket = "apitool-test-exports",
                AccessKey = "minioadmin",
                SecretKey = "minioadmin",
                UsePathStyle = true,
                Region = "us-east-1",
            },
        });

        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);

        await FluentActions.Awaiting(() => store.GetAsync($"no-such/{Guid.NewGuid():N}", default))
            .Should().ThrowAsync<KeyNotFoundException>();
    }

    [SkippableFact]
    [Trait("Category", "minio-integration")]
    public async Task MinIO_non_seekable_stream_put_then_get_round_trip()
    {
        Skip.IfNot(MinioAvailable, "MINIO_ENDPOINT not set — skipping MinIO integration test");

        var endpoint = Environment.GetEnvironmentVariable("MINIO_ENDPOINT")!;
        var opts = Options.Create(new ObjectStoreOptions
        {
            Provider = "s3",
            S3 = new ObjectStoreOptions.S3Options
            {
                Endpoint = endpoint,
                Bucket = "apitool-test-exports",
                AccessKey = "minioadmin",
                SecretKey = "minioadmin",
                UsePathStyle = true,
                Region = "us-east-1",
            },
        });

        await using var store = new S3ObjectStore(opts, NullLogger<S3ObjectStore>.Instance);

        var key = $"integration-test/{Guid.NewGuid():N}.bin";
        var payload = new byte[(5 * 1024 * 1024) + 257];
        for (var i = 0; i < payload.Length; i++)
            payload[i] = (byte)(i % 251);

        await using var content = new NonSeekableReadStream(new MemoryStream(payload));
        await store.PutAsync(key, content, "application/octet-stream", default);

        await using var downloaded = await store.GetAsync(key, default);
        using var copy = new MemoryStream();
        await downloaded.CopyToAsync(copy);
        copy.ToArray().Should().Equal(payload);
    }

    private sealed class NonSeekableReadStream(Stream inner) : Stream
    {
        public override bool CanRead => inner.CanRead;
        public override bool CanSeek => false;
        public override bool CanWrite => false;
        public override long Length => throw new NotSupportedException();
        public override long Position
        {
            get => throw new NotSupportedException();
            set => throw new NotSupportedException();
        }

        public override int Read(byte[] buffer, int offset, int count) =>
            inner.Read(buffer, offset, count);

        public override ValueTask<int> ReadAsync(
            Memory<byte> buffer,
            CancellationToken cancellationToken = default) =>
            inner.ReadAsync(buffer, cancellationToken);

        public override void Flush() { }
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) => throw new NotSupportedException();

        protected override void Dispose(bool disposing)
        {
            if (disposing)
                inner.Dispose();
            base.Dispose(disposing);
        }

        public override async ValueTask DisposeAsync()
        {
            await inner.DisposeAsync();
            GC.SuppressFinalize(this);
        }
    }
}
