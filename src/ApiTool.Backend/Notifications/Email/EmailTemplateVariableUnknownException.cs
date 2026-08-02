namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Thrown when a caller passes a variable name not declared in the template manifest.
/// Raised by <see cref="SendGridSmtpSender.SendTemplateAsync"/> before any HTTP call is made,
/// defending against the template-injection / controller-leaks-user-keys bug class.
/// Per docs/SPECIFICATION.md:8941-8942 variable matching is case-sensitive.
/// </summary>
public sealed class EmailTemplateVariableUnknownException : Exception
{
    /// <summary>The slug whose manifest rejected the unknown variables.</summary>
    public string Slug { get; }

    /// <summary>The variable names that were not declared in the manifest.</summary>
    public IReadOnlyList<string> UnknownVariables { get; }

    /// <inheritdoc cref="EmailTemplateVariableUnknownException"/>
    public EmailTemplateVariableUnknownException(string slug, IReadOnlyList<string> unknown)
        : base($"Template '{slug}' rejects unknown variable(s): {string.Join(", ", unknown)}.")
    {
        Slug = slug;
        UnknownVariables = unknown;
    }
}
