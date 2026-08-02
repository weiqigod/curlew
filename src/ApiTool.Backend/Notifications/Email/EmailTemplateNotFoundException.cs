namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Thrown when no SendGrid template ID is configured for the requested slug.
/// Raised by <see cref="SendGridSmtpSender.SendTemplateAsync"/> before any HTTP call is made.
/// </summary>
public sealed class EmailTemplateNotFoundException : Exception
{
    /// <summary>The slug that had no configured template ID.</summary>
    public string Slug { get; }

    /// <inheritdoc cref="EmailTemplateNotFoundException"/>
    public EmailTemplateNotFoundException(string slug)
        : base($"No SendGrid template id configured for slug '{slug}' "
              + $"(set APITOOL__SENDGRID__TEMPLATES__{slug.ToUpperInvariant()}).")
    {
        Slug = slug;
    }
}
