// Refs docs/SPECIFICATION.md:8924-8953 (email notification contract).
namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// A queued email send request. Consumed by EmailQueueProcessor (M14-014).
/// Carries the addressee, template slug, and the template variables that the
/// processor will merge into the email body.
/// Refs docs/SPECIFICATION.md:8924-8953.
/// </summary>
public sealed record EmailMessage(
    string To,
    string TemplateSlug,
    IReadOnlyDictionary<string, string> Variables,
    DateTimeOffset EnqueuedAt);
