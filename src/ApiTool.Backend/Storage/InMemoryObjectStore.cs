using System.Collections.Concurrent;

namespace ApiTool.Backend.Storage;

/// <summary>
/// Test-only <see cref="IObjectStore"/> backed by a <see cref="ConcurrentDictionary{TKey,TValue}"/>.
/// Never registered in Production. The signed URL returned is a local HTTP URL that can be served
/// by <see cref="InMemoryObjectStoreEndpoint"/> when wired into a test host.
/// </summary>
public sealed class InMemoryObjectStore : IObjectStore
{
    private readonly ConcurrentDictionary<string, byte[]> _store = new(StringComparer.Ordinal);

    /// <inheritdoc/>
    public async Task PutAsync(string key, Stream content, string contentType, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);
        ArgumentNullException.ThrowIfNull(content);

        using var ms = new MemoryStream();
        await content.CopyToAsync(ms, ct);
        _store[key] = ms.ToArray();
    }

    /// <inheritdoc/>
    /// <remarks>
    /// The signed URL uses the path format <c>/api/v1/internal/object-store/{key}?exp={unix}</c>
    /// so that the <see cref="InMemoryObjectStoreEndpoint"/> can serve it directly in the test host.
    /// </remarks>
    public Task<Uri> GetSignedUrlAsync(string key, TimeSpan ttl, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        var exp = DateTimeOffset.UtcNow.Add(ttl).ToUnixTimeSeconds();
        // Use the in-process test endpoint path; the test host's HttpClient will resolve this correctly.
        var url = new Uri($"http://localhost/api/v1/internal/object-store/{key}?exp={exp}");
        return Task.FromResult(url);
    }

    /// <inheritdoc/>
    public Task<Stream> GetAsync(string key, CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(key);

        if (!_store.TryGetValue(key, out var bytes))
            throw new KeyNotFoundException($"Object key '{key}' not found in InMemoryObjectStore.");

        Stream stream = new MemoryStream(bytes, writable: false);
        return Task.FromResult(stream);
    }
}
