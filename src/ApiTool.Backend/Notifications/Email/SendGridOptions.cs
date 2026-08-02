// Spec refs: docs/SPECIFICATION.md:8944-8951 (env-var → option binding).
namespace ApiTool.Backend.Notifications.Email;

/// <summary>Strongly-typed configuration for the SendGrid integration.</summary>
public sealed class SendGridOptions
{
    /// <summary>The IConfiguration section key this options class is bound from.</summary>
    public const string Section = "ApiTool:SendGrid";

    /// <summary>"fake" (default) or "live". Live requires <see cref="ApiKey"/>.</summary>
    public string Mode { get; set; } = "fake";

    /// <summary>SendGrid API key. Required when <see cref="Mode"/> is "live".</summary>
    public string ApiKey { get; set; } = string.Empty;

    /// <summary>Sender email address.</summary>
    public string FromEmail { get; set; } = "noreply@apitool.dev";

    /// <summary>Sender display name.</summary>
    public string FromName { get; set; } = "ApiTool";

    /// <summary>
    /// Per-template SendGrid Dynamic Template IDs.
    /// Bound from <c>APITOOL__SENDGRID__TEMPLATES__&lt;SLUG&gt;</c> env vars.
    /// Lookup is case-insensitive on the slug key.
    /// </summary>
    public Dictionary<string, string> Templates { get; set; } =
        new(StringComparer.OrdinalIgnoreCase);
}
