using ApiTool.Backend.Subscriptions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies StripeGateway startup validation and construction.</summary>
public sealed class StripeGatewayConstructionTests
{
    [Fact]
    public void Throws_when_live_mode_with_empty_api_key()
    {
        var opts = Options.Create(new StripeOptions { Mode = "live", ApiKey = "" });
        var act = () => new StripeGateway(opts, NullLogger<StripeGateway>.Instance);
        act.Should().Throw<InvalidOperationException>().WithMessage("*ApiTool:Stripe:Mode=live*");
    }

    [Fact]
    public void Constructs_when_live_mode_with_api_key_and_api_base()
    {
        var opts = Options.Create(new StripeOptions
        {
            Mode = "live", ApiKey = "sk_test_abc", ApiBase = "http://localhost:12111"
        });
        var act = () => new StripeGateway(opts, NullLogger<StripeGateway>.Instance);
        act.Should().NotThrow();
    }
}
