using System.Net;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Notifications;
using FluentAssertions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Parameterized validator that drives every slug in <see cref="EmailTemplateInventory.Slugs"/>
/// through manifest round-trip, MJML compile, variable-allowlist, and placeholder-copy checks.
/// Filter: FullyQualifiedName~EmailTemplateInventory
/// </summary>
public sealed class EmailTemplateInventoryManifestTests
{
    public static IEnumerable<object[]> AllSlugs() =>
        EmailTemplateInventory.Slugs.Select(s => new object[] { s });

    /// <summary>
    /// Spec-prescribed variable set for each slug.
    /// M14 source: docs/SPECIFICATION.md:8926-8933.
    /// M16 additions: docs/SPECIFICATION.md:8586-8587.
    /// </summary>
    private static readonly Dictionary<string, string[]> ExpectedVariables = new()
    {
        ["email_verification"]            = ["first_name", "verification_url"],
        ["auth_device_code"]              = ["user_code", "verification_url", "expires_in_minutes"],
        ["billing_receipt"]               = ["first_name", "billing_period", "amount_total", "invoice_url"],
        ["billing_payment_failed"]        = ["first_name", "amount_total", "update_payment_url", "attempt_count"],
        ["billing_subscription_canceled"] = ["first_name", "tier", "effective_date"],
        ["account_security_alert"]        = ["first_name", "event_time", "event_ip", "relogin_url"],
        // M16 additions (spec :8586-8587):
        ["password_reset"]                = ["user_email", "reset_url", "expires_at_local", "requester_ip", "requester_ua"],
        ["trial_expiring"]                = ["user_email", "feature", "expires_at_local", "upgrade_url"],
        // M18-005 additions (v4-5 — GDPR deletion state machine):
        ["account_deletion_initiated"]    = ["user_email", "cancel_url", "finalizes_at_local"],
        ["account_deletion_completed"]    = ["user_email", "deleted_at_local"],
    };

    private static string GetRepoRoot() => EmailTemplateInventoryTests.GetRepoRoot();

    [Theory, MemberData(nameof(AllSlugs))]
    public void Manifest_round_trips_loader(string slug)
    {
        var loader = new EmailTemplateLoader(Path.Combine(GetRepoRoot(), "templates", "email"));
        var m = loader.LoadManifest(slug);
        m.Slug.Should().Be(slug);
        m.Subject.Should().NotBeNullOrWhiteSpace(
            because: $"manifest for {slug} must have a non-empty subject");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void Mjml_compiles_to_non_empty_html(string slug)
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var html = new EmailPreviewRenderer(
            new EmailTemplateLoader(root),
            new MjmlNetCompiler()).Render(slug);
        html.Length.Should().BeGreaterThan(1_000,
            because: $"{slug} must produce a non-trivial HTML document");
        html.ToLower().Should().Contain("<!doctype html>",
            because: $"{slug} compiled HTML must be a full HTML document");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void TestData_keys_match_variables_exactly(string slug)
    {
        var m = new EmailTemplateLoader(Path.Combine(GetRepoRoot(), "templates", "email"))
            .LoadManifest(slug);
        m.TestData.Keys.Should().BeEquivalentTo(m.Variables.Keys,
            because: $"every declared variable in {slug} needs a test_data default and vice versa");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void Manifest_variables_match_spec_allowlist(string slug)
    {
        var m = new EmailTemplateLoader(Path.Combine(GetRepoRoot(), "templates", "email"))
            .LoadManifest(slug);
        m.Variables.Keys.Should().BeEquivalentTo(ExpectedVariables[slug],
            because: $"the spec at :8926-8933 prescribes the exact variable set for {slug}");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void SendGridSmtpSender_rejects_unknown_variable(string slug)
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var loader = new EmailTemplateLoader(root);
        var opts = Options.Create(new SendGridOptions
        {
            Mode = "live",
            ApiKey = "SG.fake",
            Templates = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                [slug] = "d-fake-id"
            },
        });

        // Stub client that throws if any HTTP call is made — the allowlist exception
        // must fire BEFORE any network call.
        var http = new NeverCalledSendGridClient();
        var sender = new SendGridSmtpSender(opts, loader, NullLogger<SendGridSmtpSender>.Instance, http);

        var firstAllowedVar = loader.LoadManifest(slug).Variables.Keys.First();
        var vars = new Dictionary<string, string>
        {
            [firstAllowedVar] = "ok",
            ["__definitely_not_a_variable"] = "should-fail",
        };

        var act = () => sender.SendTemplateAsync("alex@example.com", slug, vars, default)
            .GetAwaiter().GetResult();
        act.Should().Throw<EmailTemplateVariableUnknownException>(
            because: $"{slug}: unknown variable '__definitely_not_a_variable' must be rejected before HTTP call");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void Mjml_starts_with_placeholder_copy_header(string slug)
    {
        var loader = new EmailTemplateLoader(Path.Combine(GetRepoRoot(), "templates", "email"));
        loader.LoadMjml(slug).Should().Contain(
            "placeholder copy; product/design polish in a later non-M14 commit",
            because: $"Open Decision #8 requires this header on every M14 template ({slug})");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void TestData_keys_are_subset_of_variables(string slug)
    {
        var m = new EmailTemplateLoader(Path.Combine(GetRepoRoot(), "templates", "email"))
            .LoadManifest(slug);
        m.TestData.Keys.Should().BeSubsetOf(m.Variables.Keys,
            because: $"test_data in {slug} must only reference declared variables");
    }

    [Theory, MemberData(nameof(AllSlugs))]
    public void Mjml_uses_all_declared_variables(string slug)
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        var loader = new EmailTemplateLoader(root);
        var mjml = loader.LoadMjml(slug);
        var manifest = loader.LoadManifest(slug);

        foreach (var variable in manifest.Variables.Keys)
        {
            mjml.Should().Contain($"{{{{{variable}}}}}",
                because: $"template '{slug}' declares variable '{variable}' in its manifest but never references it in the MJML body");
        }
    }
}

/// <summary>
/// Stub <see cref="SendGrid.ISendGridClient"/> that throws on any invocation.
/// The allowlist check in <see cref="SendGridSmtpSender.SendTemplateAsync"/> must
/// raise <see cref="EmailTemplateVariableUnknownException"/> before any HTTP call.
/// </summary>
internal sealed class NeverCalledSendGridClient : SendGrid.ISendGridClient
{
    private static T Fail<T>() =>
        throw new InvalidOperationException(
            "HTTP call should not be made — variable allowlist must reject first.");

    public string UrlPath { get; set; } = string.Empty;
    public string Version { get; set; } = string.Empty;
    public string MediaType { get; set; } = string.Empty;

    public System.Net.Http.Headers.AuthenticationHeaderValue AddAuthorization(
        KeyValuePair<string, string> header)
        => new("Bearer", "fake");

    public Task<SendGrid.Response> MakeRequest(
        System.Net.Http.HttpRequestMessage request,
        System.Threading.CancellationToken cancellationToken = default) => Fail<Task<SendGrid.Response>>();

    public Task<SendGrid.Response> RequestAsync(
        SendGrid.BaseClient.Method method,
        string requestBody = null!,
        string queryParams = null!,
        string urlPath = null!,
        System.Threading.CancellationToken cancellationToken = default) => Fail<Task<SendGrid.Response>>();

    public Task<SendGrid.Response> SendEmailAsync(
        SendGrid.Helpers.Mail.SendGridMessage msg,
        System.Threading.CancellationToken cancellationToken = default) => Fail<Task<SendGrid.Response>>();
}
