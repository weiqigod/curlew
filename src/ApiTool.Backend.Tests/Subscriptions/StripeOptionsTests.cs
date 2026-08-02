using ApiTool.Backend.Subscriptions;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies StripeOptions binds correctly from configuration.</summary>
public sealed class StripeOptionsTests
{
    [Fact]
    public void Defaults_to_fake_mode_with_empty_api_key()
    {
        var options = new StripeOptions();
        options.Mode.Should().Be("fake");
        options.ApiKey.Should().BeEmpty();
        options.ApiBase.Should().BeNull();
    }

    [Fact]
    public void Bound_from_configuration_with_section_keys()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:Stripe:Mode"] = "live",
                ["ApiTool:Stripe:ApiKey"] = "sk_test_abc",
                ["ApiTool:Stripe:ApiBase"] = "http://localhost:12111",
            }).Build();
        var options = new StripeOptions();
        cfg.GetSection(StripeOptions.Section).Bind(options);
        options.Mode.Should().Be("live");
        options.ApiKey.Should().Be("sk_test_abc");
        options.ApiBase.Should().Be("http://localhost:12111");
    }
}
