namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A minimal <see cref="IHttpClientFactory"/> that always returns an <see cref="HttpClient"/>
/// backed by the given <see cref="HttpMessageHandler"/>.
/// </summary>
public sealed class FakeHttpClientFactory : IHttpClientFactory
{
    private readonly HttpMessageHandler _handler;
    private readonly string? _baseAddress;

    /// <summary>Creates a factory using a <see cref="FakeHttpMessageHandler"/>.</summary>
    /// <param name="handler">The message handler to use.</param>
    /// <param name="baseAddress">Optional base address. Defaults to "https://api.github.com" when null.</param>
    public FakeHttpClientFactory(FakeHttpMessageHandler handler, string? baseAddress = null)
    {
        _handler = handler;
        _baseAddress = baseAddress;
    }

    /// <summary>Creates a factory using a <see cref="QueuedFakeHttpMessageHandler"/>.</summary>
    /// <param name="handler">The message handler to use.</param>
    /// <param name="baseAddress">Optional base address. Defaults to "https://api.github.com" when null.</param>
    public FakeHttpClientFactory(QueuedFakeHttpMessageHandler handler, string? baseAddress = null)
    {
        _handler = handler;
        _baseAddress = baseAddress;
    }

    /// <inheritdoc/>
    public HttpClient CreateClient(string name)
    {
        var client = new HttpClient(_handler, disposeHandler: false);
        var addr = _baseAddress ?? "https://api.github.com";
        if (!string.IsNullOrEmpty(addr))
            client.BaseAddress = new Uri(addr);
        return client;
    }
}
