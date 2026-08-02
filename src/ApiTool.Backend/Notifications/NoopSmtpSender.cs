using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Notifications;

/// <summary>
/// No-op implementation of <see cref="ISmtpSender"/> used in development and testing.
/// Logs the intended email but does not actually send it.
/// </summary>
public sealed class NoopSmtpSender(ILogger<NoopSmtpSender> logger) : ISmtpSender
{
    /// <inheritdoc/>
    public Task SendAsync(string toAddress, string subject, string body, CancellationToken ct)
    {
        logger.LogInformation(
            "NoopSmtpSender: would send email to {ToAddress} with subject '{Subject}'",
            toAddress, subject);
        return Task.CompletedTask;
    }

    /// <inheritdoc/>
    public Task SendTemplateAsync(
        string toAddress,
        string slug,
        IDictionary<string, string> variables,
        CancellationToken ct)
    {
        logger.LogInformation(
            "NoopSmtpSender: would send template email to {ToAddress} with slug '{Slug}'",
            toAddress, slug);
        return Task.CompletedTask;
    }
}
