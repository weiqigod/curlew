using System.Net;
using System.Net.Http.Headers;
using System.Text;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A minimal <see cref="HttpMessageHandler"/> that returns a pre-configured response.
/// Records the last request body for assertion in tests.
/// </summary>
public sealed class FakeHttpMessageHandler(
    HttpStatusCode statusCode,
    string responseBody,
    string contentType = "application/json") : HttpMessageHandler
{
    private int _requestCount;

    /// <summary>Gets the number of HTTP requests received.</summary>
    public int RequestCount => _requestCount;

    /// <summary>Gets the body of the last HTTP request received, or empty string if none.</summary>
    public string LastRequestBody { get; private set; } = string.Empty;

    /// <summary>Gets the Authorization header value of the last request.</summary>
    public string? LastAuthorizationHeader { get; private set; }

    /// <summary>
    /// Gets all request headers (both standard and non-standard) from the last request.
    /// Keyed by header name; values are comma-joined when multi-value.
    /// </summary>
    public Dictionary<string, string> LastRequestHeaders { get; private set; } = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>Gets or sets the headers to include in responses (for rate-limit simulation).</summary>
    public Dictionary<string, string> ResponseHeaders { get; } = new();

    protected override async Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        Interlocked.Increment(ref _requestCount);
        LastAuthorizationHeader = request.Headers.Authorization?.ToString();

        // Capture all request headers (standard + non-standard)
        var captured = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        foreach (var (key, values) in request.Headers)
            captured[key] = string.Join(", ", values);
        if (request.Content is not null)
            foreach (var (key, values) in request.Content.Headers)
                captured[key] = string.Join(", ", values);
        LastRequestHeaders = captured;

        if (request.Content is not null)
            LastRequestBody = await request.Content.ReadAsStringAsync(cancellationToken);

        var response = new HttpResponseMessage(statusCode)
        {
            Content = new StringContent(responseBody, Encoding.UTF8, contentType),
        };

        foreach (var (key, value) in ResponseHeaders)
            response.Headers.TryAddWithoutValidation(key, value);

        return response;
    }
}
