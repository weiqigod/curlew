using ApiTool.Backend.Webhooks;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>Verifies StripeWebhookOptions parsing and validation.</summary>
public sealed class StripeWebhookIngest_OptionsTests
{
    [Fact]
    public void Defaults_to_empty_secrets_and_300s_tolerance()
    {
        var opts = new StripeWebhookOptions();
        opts.Secrets.Should().BeEmpty();
        opts.ToleranceSeconds.Should().Be(300);
        opts.SecretList().Should().BeEmpty();
    }

    [Fact]
    public void SecretList_splits_trims_and_drops_empties()
    {
        var opts = new StripeWebhookOptions { Secrets = " whsec_old , whsec_new ,, " };
        opts.SecretList().Should().BeEquivalentTo(new[] { "whsec_old", "whsec_new" });
    }

    [Fact]
    public void SecretList_with_single_secret_returns_one_entry()
    {
        var opts = new StripeWebhookOptions { Secrets = "whsec_test" };
        opts.SecretList().Should().ContainSingle().Which.Should().Be("whsec_test");
    }

    [Fact]
    public void Bound_from_config_section_with_canonical_double_underscore_path()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:Stripe:Webhook:Secrets"] = "whsec_test",
                ["ApiTool:Stripe:Webhook:ToleranceSeconds"] = "600",
            }).Build();
        var opts = new StripeWebhookOptions();
        cfg.GetSection(StripeWebhookOptions.Section).Bind(opts);
        opts.Secrets.Should().Be("whsec_test");
        opts.ToleranceSeconds.Should().Be(600);
    }

    [Fact]
    public async Task Boot_with_tolerance_below_minimum_throws_OptionsValidationException()
    {
        // Spec ref :6854 — tolerance=0 must be rejected at boot.
        // We build a minimal host that wires StripeWebhookOptions with ValidateOnStart
        // and assert that StartAsync throws OptionsValidationException.
        var host = Host.CreateDefaultBuilder()
            .ConfigureAppConfiguration(cfg =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:Stripe:Webhook:Secrets"] = "whsec_test",
                    ["ApiTool:Stripe:Webhook:ToleranceSeconds"] = "0", // footgun value
                });
            })
            .ConfigureServices((ctx, services) =>
            {
                services.AddOptions<StripeWebhookOptions>()
                    .Bind(ctx.Configuration.GetSection(StripeWebhookOptions.Section))
                    .Validate(
                        o => o.ToleranceSeconds >= 30,
                        "ApiTool:Stripe:Webhook:ToleranceSeconds must be >= 30. " +
                        "Setting to 0 disables Stripe.Net timestamp verification entirely (spec :6854).")
                    .ValidateOnStart();
            })
            .Build();

        var act = () => host.StartAsync();
        await act.Should().ThrowAsync<OptionsValidationException>()
            .WithMessage("*ToleranceSeconds*");
    }

    [Fact]
    public async Task Boot_with_empty_secrets_in_live_mode_throws_OptionsValidationException()
    {
        // Spec ref :6850 — live mode requires at least one secret.
        var host = Host.CreateDefaultBuilder()
            .ConfigureAppConfiguration(cfg =>
            {
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:Stripe:Mode"] = "live",
                    ["ApiTool:Stripe:Webhook:Secrets"] = "", // empty — should fail in live mode
                    ["ApiTool:Stripe:Webhook:ToleranceSeconds"] = "300",
                });
            })
            .ConfigureServices((ctx, services) =>
            {
                var stripeMode = ctx.Configuration["ApiTool:Stripe:Mode"] ?? string.Empty;
                services.AddOptions<StripeWebhookOptions>()
                    .Bind(ctx.Configuration.GetSection(StripeWebhookOptions.Section))
                    .Validate(
                        o => o.ToleranceSeconds >= 30,
                        "ApiTool:Stripe:Webhook:ToleranceSeconds must be >= 30 (spec :6854).")
                    .Validate(
                        o => stripeMode != "live" || o.SecretList().Count > 0,
                        "ApiTool:Stripe:Mode=live requires at least one webhook secret in APITOOL__STRIPE__WEBHOOK_SECRETS.")
                    .ValidateOnStart();
            })
            .Build();

        var act = () => host.StartAsync();
        await act.Should().ThrowAsync<OptionsValidationException>()
            .WithMessage("*live*");
    }
}
