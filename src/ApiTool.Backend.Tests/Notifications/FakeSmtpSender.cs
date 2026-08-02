using ApiTool.Backend.Notifications;

namespace ApiTool.Backend.Tests.Notifications;

/// <summary>
/// Test double for <see cref="ISmtpSender"/>. Records all send calls.
/// </summary>
public sealed class FakeSmtpSender : ISmtpSender
{
    private readonly List<(string To, string Subject, string Body)> _sends = [];

    /// <summary>All recorded sends in order.</summary>
    public IReadOnlyList<(string To, string Subject, string Body)> Sends => _sends;

    private readonly List<(string To, string Slug, IDictionary<string, string> Variables)> _templateSends = [];

    /// <summary>All recorded template sends in order.</summary>
    public IReadOnlyList<(string To, string Slug, IDictionary<string, string> Variables)> TemplateSends
        => _templateSends;

    /// <inheritdoc/>
    public Task SendAsync(string toAddress, string subject, string body, CancellationToken ct)
    {
        _sends.Add((toAddress, subject, body));
        return Task.CompletedTask;
    }

    /// <inheritdoc/>
    public Task SendTemplateAsync(
        string toAddress,
        string slug,
        IDictionary<string, string> variables,
        CancellationToken ct)
    {
        _templateSends.Add((toAddress, slug, new Dictionary<string, string>(variables)));
        return Task.CompletedTask;
    }
}
