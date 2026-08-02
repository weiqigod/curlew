using System.Net;
using System.Text;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A <see cref="HttpMessageHandler"/> that dequeues pre-configured responses in order.
/// Useful for tests where sequential calls return different results (e.g. idempotency GET then no-POST).
/// </summary>
public sealed class QueuedFakeHttpMessageHandler : HttpMessageHandler
{
    private readonly Queue<(HttpStatusCode Status, string Body, string ContentType)> _queue = new();

    /// <summary>Gets the number of HTTP requests received.</summary>
    public int RequestCount { get; private set; }

    /// <summary>Gets the URI of the last request received.</summary>
    public Uri? LastRequestUri { get; private set; }

    /// <summary>Gets the HTTP method of the last request received.</summary>
    public HttpMethod? LastRequestMethod { get; private set; }

    /// <summary>Enqueues a response to be returned on the next call.</summary>
    public QueuedFakeHttpMessageHandler Enqueue(HttpStatusCode status, string body, string contentType = "application/json")
    {
        _queue.Enqueue((status, body, contentType));
        return this;
    }

    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken cancellationToken)
    {
        RequestCount++;
        LastRequestUri = request.RequestUri;
        LastRequestMethod = request.Method;

        if (_queue.Count == 0)
            throw new InvalidOperationException($"QueuedFakeHttpMessageHandler: no more responses queued (request #{RequestCount}: {request.Method} {request.RequestUri})");

        var (status, body, contentType) = _queue.Dequeue();
        var response = new HttpResponseMessage(status)
        {
            Content = new StringContent(body, Encoding.UTF8, contentType),
        };
        return Task.FromResult(response);
    }
}
