// Tests for TierConfig — static tier→(features, request_limit) lookup.
using ApiTool.Backend.Licensing.Tokens;

namespace ApiTool.Backend.Tests.Licensing.Tokens;

public sealed class TierConfigTests
{
    [Theory]
    [InlineData("free",          0, 1000)]
    [InlineData("professional",  3, 100_000)]
    [InlineData("team",          5, 1_000_000)]
    [InlineData("enterprise",    7, int.MaxValue)]
    public void For_returns_expected_features_and_request_limit(string tier, int minFeatureCount, int requestLimit)
    {
        var (features, limit) = TierConfig.For(tier);
        features.Should().HaveCountGreaterOrEqualTo(minFeatureCount);
        limit.Should().Be(requestLimit);
    }

    [Fact]
    public void For_free_returns_empty_features()
    {
        var (features, _) = TierConfig.For("free");
        features.Should().BeEmpty();
    }

    [Fact]
    public void For_unknown_tier_returns_free_defaults()
    {
        var (features, limit) = TierConfig.For("unknown_tier");
        features.Should().BeEmpty();
        limit.Should().Be(1000);
    }
}
