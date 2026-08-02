// Spec refs: docs/SPECIFICATION.md:8903-8922 (manifest schema).
namespace ApiTool.Backend.Notifications.Email;

/// <summary>Schema of the <c>&lt;slug&gt;.json</c> manifest sidecar for an email template.</summary>
public sealed record EmailTemplateManifest(
    string Slug,
    string Subject,
    IReadOnlyDictionary<string, string> Variables,
    IReadOnlyDictionary<string, string> TestData);
