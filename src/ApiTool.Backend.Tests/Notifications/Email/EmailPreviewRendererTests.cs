using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class EmailPreviewRendererTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), $"ev_{Guid.NewGuid():N}");

    public EmailPreviewRendererTests()
    {
        Directory.CreateDirectory(_root);
        // Write a minimum MJML fixture with Handlebars tokens.
        File.WriteAllText(Path.Combine(_root, "x.mjml"), """
            <mjml>
              <mj-body>
                <mj-section>
                  <mj-column>
                    <mj-text>Hello {{first_name}}!</mj-text>
                    <mj-text>Click <a href="{{verification_url}}">here</a>.</mj-text>
                  </mj-column>
                </mj-section>
              </mj-body>
            </mjml>
            """);
        File.WriteAllText(Path.Combine(_root, "x.json"), """
            {
              "slug": "x",
              "subject": "S",
              "variables": { "first_name": "string", "verification_url": "string" },
              "test_data": { "first_name": "Alex", "verification_url": "https://example.com/verify" }
            }
            """);

        // XSS template: variable contains a script tag.
        File.WriteAllText(Path.Combine(_root, "xss.mjml"), """
            <mjml>
              <mj-body>
                <mj-section>
                  <mj-column>
                    <mj-text>{{payload}}</mj-text>
                  </mj-column>
                </mj-section>
              </mj-body>
            </mjml>
            """);
        File.WriteAllText(Path.Combine(_root, "xss.json"), """
            {
              "slug": "xss",
              "subject": "XSS test",
              "variables": { "payload": "string" },
              "test_data": { "payload": "<script>alert(1)</script>" }
            }
            """);
    }

    public void Dispose()
    {
        try { Directory.Delete(_root, true); } catch { /* ignore */ }
    }

    private EmailPreviewRenderer MakeRenderer()
        => new(new EmailTemplateLoader(_root), new MjmlNetCompiler());

    [Fact]
    public void Render_returns_html_with_substituted_test_data()
    {
        var html = MakeRenderer().Render("x");
        html.Should().Contain("Alex");
        html.Should().Contain("https://example.com/verify");
        html.Length.Should().BeGreaterThan(1_000);
    }

    [Fact]
    public void Render_produces_full_html_document()
    {
        var html = MakeRenderer().Render("x");
        html.Should().Contain("<!doctype html>", because: "MJML emits a full HTML document");
    }

    [Fact]
    public void Render_html_escapes_xss_in_variables()
    {
        var html = MakeRenderer().Render("xss");
        html.Should().Contain("&lt;script&gt;",
            because: "Handlebars HTML-escapes by default to prevent XSS");
        html.Should().NotContain("<script>alert(1)</script>");
    }
}
