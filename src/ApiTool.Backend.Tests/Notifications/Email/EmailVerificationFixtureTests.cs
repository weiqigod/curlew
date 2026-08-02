using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Contract tests for the email_verification fixture template that ships in M14-014.
/// Uses the real files at templates/email/email_verification.{mjml,json}.
/// </summary>
public sealed class EmailVerificationFixtureTests
{
    private static string GetRepoRoot()
    {
        // Walk up from AppContext.BaseDirectory until we find the templates/email directory.
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir is not null)
        {
            if (Directory.Exists(Path.Combine(dir.FullName, "templates", "email")))
                return dir.FullName;
            dir = dir.Parent;
        }

        throw new InvalidOperationException(
            "Could not locate repo root containing templates/email from " + AppContext.BaseDirectory);
    }

    [Fact]
    public void Fixture_template_renders_to_more_than_1000_bytes()
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var html = new EmailPreviewRenderer(new EmailTemplateLoader(root), new MjmlNetCompiler())
            .Render("email_verification");
        html.Length.Should().BeGreaterThan(1_000,
            because: "the fixture must produce a non-trivial HTML document");
        html.Should().Contain("Alex");
        html.Should().Contain("https://app.apitool.dev/verify?t=test123");
    }

    [Fact]
    public void Fixture_manifest_test_data_keys_are_subset_of_variables()
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var manifest = new EmailTemplateLoader(root).LoadManifest("email_verification");
        manifest.TestData.Keys.Should().BeSubsetOf(manifest.Variables.Keys,
            because: "test_data must only reference declared variables");
    }

    [Fact]
    public void Fixture_manifest_slug_matches_filename()
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var manifest = new EmailTemplateLoader(root).LoadManifest("email_verification");
        manifest.Slug.Should().Be("email_verification");
    }

    [Fact]
    public void Fixture_manifest_has_both_required_variables()
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var manifest = new EmailTemplateLoader(root).LoadManifest("email_verification");
        manifest.Variables.Should().ContainKey("first_name");
        manifest.Variables.Should().ContainKey("verification_url");
    }
}
