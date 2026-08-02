namespace ApiTool.Backend.Notifications;

/// <summary>Seam for sending notification emails.</summary>
public interface ISmtpSender
{
    /// <summary>
    /// Sends an email notification.
    /// </summary>
    /// <param name="toAddress">The recipient email address.</param>
    /// <param name="subject">Email subject line.</param>
    /// <param name="body">Plain-text or HTML email body.</param>
    /// <param name="ct">Cancellation token.</param>
    Task SendAsync(string toAddress, string subject, string body, CancellationToken ct);

    /// <summary>
    /// Sends an email via a SendGrid Dynamic Template.
    /// Variables are validated against the template manifest at send time;
    /// unknown keys cause rejection to defend against template-injection
    /// (docs/SPECIFICATION.md:8941-8942).
    /// </summary>
    /// <param name="toAddress">The recipient email address.</param>
    /// <param name="slug">The template slug (e.g. "email_verification").</param>
    /// <param name="variables">Template variables matching the manifest's declared keys.</param>
    /// <param name="ct">Cancellation token.</param>
    Task SendTemplateAsync(
        string toAddress,
        string slug,
        IDictionary<string, string> variables,
        CancellationToken ct);
}
