using Amazon;
using Amazon.S3;
using Amazon.S3.Model;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Storage;

/// <summary>
/// <see cref="IObjectStore"/> implementation backed by Amazon S3 or an S3-compatible store
/// (e.g. MinIO).  Configured via <see cref="ObjectStoreOptions.S3Options"/>.
/// </summary>
public sealed class S3ObjectStore : IObjectStore, IAsyncDisposable
{
    // S3 requires every non-final multipart part to be at least 5 MiB. Keeping
    // one part in memory lets callers stream arbitrarily large exports without
    // forcing the entire object into a seekable buffer.
    private const int MultipartPartSize = 5 * 1024 * 1024;

    private readonly IAmazonS3 _client;
    private readonly string _bucket;
    private readonly Protocol _signedUrlProtocol;
    private readonly ILogger<S3ObjectStore> _logger;
    private int _bucketEnsured; // 0 = not yet, 1 = done

    /// <summary>Initialises the S3 client from the supplied options.</summary>
    public S3ObjectStore(IOptions<ObjectStoreOptions> options, ILogger<S3ObjectStore> logger)
    {
        ArgumentNullException.ThrowIfNull(options);
        var s3 = options.Value.S3;
        _bucket = s3.Bucket;
        _signedUrlProtocol = Uri.TryCreate(s3.Endpoint, UriKind.Absolute, out var endpointUri)
            && endpointUri.Scheme == Uri.UriSchemeHttp
                ? Protocol.HTTP
                : Protocol.HTTPS;
        _logger = logger;

        var config = new AmazonS3Config
        {
            ForcePathStyle = s3.UsePathStyle,
        };

        if (!string.IsNullOrEmpty(s3.Endpoint))
            config.ServiceURL = s3.Endpoint;
        else
            config.RegionEndpoint = RegionEndpoint.GetBySystemName(s3.Region);

        _client = !string.IsNullOrEmpty(s3.AccessKey) && !string.IsNullOrEmpty(s3.SecretKey)
            ? new AmazonS3Client(s3.AccessKey, s3.SecretKey, config)
            : new AmazonS3Client(config);
    }

    /// <summary>Initialises from an explicit client (used in tests).</summary>
    internal S3ObjectStore(IAmazonS3 client, string bucket, ILogger<S3ObjectStore> logger)
    {
        _client = client;
        _bucket = bucket;
        _signedUrlProtocol = Protocol.HTTPS;
        _logger = logger;
        _bucketEnsured = 1; // Tests manage their own bucket lifecycle.
    }

    /// <inheritdoc/>
    public async Task PutAsync(string key, Stream content, string contentType, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);
        ArgumentNullException.ThrowIfNull(content);

        await EnsureBucketAsync(ct);

        if (!content.CanSeek)
        {
            await PutMultipartAsync(key, content, contentType, ct);
            return;
        }

        await PutObjectAsync(key, content, contentType, ct);
    }

    private async Task PutObjectAsync(
        string key,
        Stream content,
        string contentType,
        CancellationToken ct)
    {
        var request = new PutObjectRequest
        {
            BucketName = _bucket,
            Key = key,
            InputStream = content,
            ContentType = contentType,
        };

        await _client.PutObjectAsync(request, ct);
        _logger.LogDebug("S3ObjectStore: PUT {Bucket}/{Key}", _bucket, key);
    }

    private async Task PutMultipartAsync(
        string key,
        Stream content,
        string contentType,
        CancellationToken ct)
    {
        var buffer = new byte[MultipartPartSize];
        var firstPartLength = await ReadPartAsync(content, buffer, ct);

        // S3 multipart uploads cannot contain a zero-length part.
        if (firstPartLength == 0)
        {
            await PutObjectAsync(key, new MemoryStream(Array.Empty<byte>()), contentType, ct);
            return;
        }

        var initiation = await _client.InitiateMultipartUploadAsync(
            new InitiateMultipartUploadRequest
            {
                BucketName = _bucket,
                Key = key,
                ContentType = contentType,
            },
            ct);

        try
        {
            var partETags = new List<PartETag>();
            var partNumber = 1;
            var partLength = firstPartLength;

            while (partLength > 0)
            {
                using var partStream = new MemoryStream(buffer, 0, partLength, writable: false);
                var response = await _client.UploadPartAsync(
                    new UploadPartRequest
                    {
                        BucketName = _bucket,
                        Key = key,
                        UploadId = initiation.UploadId,
                        PartNumber = partNumber,
                        PartSize = partLength,
                        InputStream = partStream,
                    },
                    ct);

                partETags.Add(new PartETag(partNumber, response.ETag));
                partNumber++;
                partLength = await ReadPartAsync(content, buffer, ct);
            }

            await _client.CompleteMultipartUploadAsync(
                new CompleteMultipartUploadRequest
                {
                    BucketName = _bucket,
                    Key = key,
                    UploadId = initiation.UploadId,
                    PartETags = partETags,
                },
                ct);

            _logger.LogDebug(
                "S3ObjectStore: multipart PUT {Bucket}/{Key} ({PartCount} parts)",
                _bucket,
                key,
                partETags.Count);
        }
        catch
        {
            try
            {
                await _client.AbortMultipartUploadAsync(
                    new AbortMultipartUploadRequest
                    {
                        BucketName = _bucket,
                        Key = key,
                        UploadId = initiation.UploadId,
                    },
                    CancellationToken.None);
            }
            catch (Exception abortException)
            {
                _logger.LogWarning(
                    abortException,
                    "S3ObjectStore: failed to abort multipart upload {UploadId} for {Bucket}/{Key}",
                    initiation.UploadId,
                    _bucket,
                    key);
            }

            throw;
        }
    }

    private static async Task<int> ReadPartAsync(
        Stream content,
        byte[] buffer,
        CancellationToken ct)
    {
        var total = 0;
        while (total < buffer.Length)
        {
            var read = await content.ReadAsync(buffer.AsMemory(total), ct);
            if (read == 0)
                break;
            total += read;
        }

        return total;
    }

    /// <inheritdoc/>
    public Task<Uri> GetSignedUrlAsync(string key, TimeSpan ttl, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        var request = new GetPreSignedUrlRequest
        {
            BucketName = _bucket,
            Key = key,
            Verb = HttpVerb.GET,
            Expires = DateTime.UtcNow.Add(ttl),
            Protocol = _signedUrlProtocol,
        };

        var url = _client.GetPreSignedURL(request);
        return Task.FromResult(new Uri(url));
    }

    /// <inheritdoc/>
    /// <remarks>
    /// Retrieves object content from S3.  Not used in the normal serving path (the signed URL
    /// directs callers to S3 directly); exposed for test-hook endpoint parity.
    /// </remarks>
    public async Task<Stream> GetAsync(string key, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        try
        {
            var response = await _client.GetObjectAsync(_bucket, key, ct);
            return response.ResponseStream;
        }
        catch (AmazonS3Exception ex) when (ex.StatusCode == System.Net.HttpStatusCode.NotFound)
        {
            throw new KeyNotFoundException($"Object key '{key}' not found in S3 bucket '{_bucket}'.", ex);
        }
    }

    // Creates the bucket if it does not yet exist.  Idempotent; called lazily on first PutAsync
    // so that production deployments (where the bucket pre-exists) pay no extra overhead.
    private async Task EnsureBucketAsync(CancellationToken ct)
    {
        if (Interlocked.Exchange(ref _bucketEnsured, 1) == 1)
            return;

        try
        {
            await _client.PutBucketAsync(new PutBucketRequest
            {
                BucketName = _bucket,
                UseClientRegion = true,
            }, ct);
            _logger.LogInformation("S3ObjectStore: bucket '{Bucket}' created.", _bucket);
        }
        catch (AmazonS3Exception ex) when (
            ex.ErrorCode is "BucketAlreadyExists" or "BucketAlreadyOwnedByYou")
        {
            // Bucket already exists — this is the expected case in production.
            _logger.LogDebug("S3ObjectStore: bucket '{Bucket}' already exists.", _bucket);
        }
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync()
    {
        _client.Dispose();
        return ValueTask.CompletedTask;
    }
}
