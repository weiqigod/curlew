using HandlebarsDotNet;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Renders an email template to HTML using the manifest's <c>test_data</c> block.
/// Used by the <c>dev email-preview</c> CLI subcommand for offline preview without a SendGrid account.
/// Per docs/SPECIFICATION.md:8915-8920 (test_data is the dev-preview substitution source).
/// </summary>
public sealed class EmailPreviewRenderer(
    EmailTemplateLoader loader,
    IMjmlCompiler compiler)
{
    /// <summary>
    /// Compiles the slug's MJML to HTML and substitutes the manifest's test_data variables.
    /// </summary>
    /// <param name="slug">The template slug (e.g. "email_verification").</param>
    /// <returns>The fully rendered HTML string.</returns>
    public string Render(string slug)
    {
        var manifest = loader.LoadManifest(slug);
        var mjml = loader.LoadMjml(slug);
        var html = compiler.Compile(mjml);

        // Use HandlebarsDotNet for local substitution — matches SendGrid's Handlebars dialect.
        // Default mode HTML-escapes output, providing XSS protection in the preview.
        var template = Handlebars.Compile(html);
        return template(manifest.TestData);
    }
}
