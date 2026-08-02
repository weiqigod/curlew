// Refs docs/SPECIFICATION.md:8577-8584 (multi-secret rotation pattern).
using ApiTool.Backend.GitHub.Webhooks;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubWebhookOptions config class.
/// </summary>
public sealed class GithubWebhookIngest_OptionsTests
{
    [Fact]
    public void Defaults_to_empty_secrets()
    {
        var opts = new GithubWebhookOptions();
        opts.Secrets.Should().BeEmpty();
        opts.SecretList().Should().BeEmpty();
    }

    [Fact]
    public void SecretList_splits_trims_and_drops_empties()
    {
        var opts = new GithubWebhookOptions { Secrets = "whsec_old , whsec_new ,, " };
        var list = opts.SecretList();
        list.Should().HaveCount(2);
        list.Should().Contain("whsec_old");
        list.Should().Contain("whsec_new");
    }

    [Fact]
    public void SecretList_with_single_secret_returns_one_entry()
    {
        var opts = new GithubWebhookOptions { Secrets = "whsec_test" };
        var list = opts.SecretList();
        list.Should().HaveCount(1);
        list[0].Should().Be("whsec_test");
    }

    [Fact]
    public void Bound_from_canonical_double_underscore_path()
    {
        var config = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:GitHub:Webhook:Secrets"] = "whsec_from_env",
            })
            .Build();

        var opts = config.GetSection(GithubWebhookOptions.Section).Get<GithubWebhookOptions>()!;
        opts.Secrets.Should().Be("whsec_from_env");
    }
}
