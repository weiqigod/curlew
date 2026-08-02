namespace ApiTool.Backend.Storage;

/// <summary>
/// Abstraction over an object/blob store used by the GDPR export builder (M18-004).
/// Implementations: <see cref="InMemoryObjectStore"/> (test), <c>S3ObjectStore</c> (self-hosted / MinIO),
/// <c>GcsObjectStore</c> (SaaS). Selected at startup via <c>ApiTool:ObjectStore:Provider</c>.
/// </summary>
public interface IObjectStore
{
    /// <summary>Writes <paramref name="content"/> to the store at the given <paramref name="key"/>.</summary>
    /// <param name="key">The object key (path-like, e.g. <c>exports/{userId}/{requestId}.json</c>).</param>
    /// <param name="content">The content stream. May be consumed only once; the caller owns disposal.</param>
    /// <param name="contentType">MIME type of the content, e.g. <c>application/json</c>.</param>
    /// <param name="ct">Cancellation token.</param>
    Task PutAsync(string key, Stream content, string contentType, CancellationToken ct);

    /// <summary>
    /// Returns a pre-signed GET URL for <paramref name="key"/> that expires after <paramref name="ttl"/>.
    /// For <see cref="InMemoryObjectStore"/> the URL is a local test-only URL.
    /// </summary>
    Task<Uri> GetSignedUrlAsync(string key, TimeSpan ttl, CancellationToken ct);

    /// <summary>
    /// Returns a readable stream for <paramref name="key"/>. Used by the in-memory test endpoint
    /// that serves downloads for the signed URL. Callers must dispose the returned stream.
    /// </summary>
    /// <exception cref="KeyNotFoundException">Thrown when <paramref name="key"/> does not exist.</exception>
    Task<Stream> GetAsync(string key, CancellationToken ct);
}
