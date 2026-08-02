using ApiTool.Backend.Notifications;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Test double for <see cref="ISmtpSender"/> focused on template sends.
/// Preferred over <see cref="FakeSmtpSender"/> in M14-014 tests where
/// <see cref="ISmtpSender.SendTemplateAsync"/> behaviour needs to be scripted.
/// </summary>
public sealed class RecordingSmtpSender : ISmtpSender
{
    private readonly List<(string To, string Slug, IDictionary<string, string> Variables)> _templateSends = [];

    /// <summary>All recorded template sends in order.</summary>
    public IReadOnlyList<(string To, string Slug, IDictionary<string, string> Variables)> TemplateSends
        => _templateSends;

    /// <summary>When set, throws this exception on the next <see cref="SendTemplateAsync"/> call.</summary>
    public Queue<Exception?> SendResults { get; } = new();

    /// <inheritdoc/>
    public Task SendAsync(string toAddress, string subject, string body, CancellationToken ct)
        => Task.CompletedTask;

    /// <inheritdoc/>
    public Task SendTemplateAsync(
        string toAddress,
        string slug,
        IDictionary<string, string> variables,
        CancellationToken ct)
    {
        _templateSends.Add((toAddress, slug, new Dictionary<string, string>(variables)));

        if (SendResults.TryDequeue(out var ex) && ex is not null)
            throw ex;

        return Task.CompletedTask;
    }
}
