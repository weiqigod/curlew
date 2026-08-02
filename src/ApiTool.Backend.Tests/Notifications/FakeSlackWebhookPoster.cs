using ApiTool.Backend.Notifications;

namespace ApiTool.Backend.Tests.Notifications;

/// <summary>
/// Test double for <see cref="ISlackWebhookPoster"/>. Records all calls and returns
/// scripted status codes (200 by default).
/// </summary>
public sealed class FakeSlackWebhookPoster : ISlackWebhookPoster
{
    private readonly Queue<int> _responseCodes = new();
    private readonly List<(string Url, string Payload)> _calls = [];

    /// <summary>All recorded (url, payload) calls in order.</summary>
    public IReadOnlyList<(string Url, string Payload)> Calls => _calls;

    /// <summary>Enqueues a response code to be returned on the next call.</summary>
    public void EnqueueStatusCode(int code) => _responseCodes.Enqueue(code);

    /// <inheritdoc/>
    public Task<int> PostAsync(string webhookUrl, string payload, CancellationToken ct)
    {
        _calls.Add((webhookUrl, payload));
        var code = _responseCodes.Count > 0 ? _responseCodes.Dequeue() : 200;
        return Task.FromResult(code);
    }
}
