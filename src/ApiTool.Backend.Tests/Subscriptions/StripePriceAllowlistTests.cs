using ApiTool.Backend.Subscriptions;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies StripePriceAllowlist only accepts documented price id prefixes.</summary>
public sealed class StripePriceAllowlistTests
{
    [Theory]
    [InlineData("price_test_solo_monthly", true)]
    [InlineData("price_test_team_yearly", true)]
    [InlineData("price_solo_monthly_9", true)]
    [InlineData("price_professional_monthly_19", true)]
    [InlineData("price_team_monthly_39_flat", true)]
    [InlineData("price_enterprise_custom", true)]
    [InlineData("", false)]
    [InlineData("price_unknown_thing", false)]
    [InlineData("price_solo", false)]
    [InlineData("not_a_price", false)]
    [InlineData("price_test_solo_monthly; DROP TABLE subscriptions;--", false)]
    public void IsAllowed_recognises_only_documented_prefixes(string priceId, bool expected)
        => StripePriceAllowlist.IsAllowed(priceId).Should().Be(expected);
}
