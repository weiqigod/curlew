using ApiTool.Backend.Subscriptions;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Verifies SubscriptionId wire-format helpers.</summary>
public sealed class SubscriptionIdTests
{
    [Theory]
    [InlineData("sub_00000000000000000000000000000001", true)]
    [InlineData("sub_badhex", false)]
    [InlineData("org_00000000000000000000000000000001", false)]
    [InlineData("", false)]
    [InlineData(null, false)]
    public void TryParse_round_trips(string? input, bool expected)
    {
        var result = SubscriptionId.TryParse(input, out _);
        result.Should().Be(expected);
    }

    [Fact]
    public void Format_then_TryParse_is_identity()
    {
        var id = Guid.NewGuid();
        var formatted = SubscriptionId.Format(id);
        SubscriptionId.TryParse(formatted, out var parsed).Should().BeTrue();
        parsed.Should().Be(id);
    }

    [Fact]
    public void Format_uses_sub_prefix()
    {
        var id = Guid.NewGuid();
        var formatted = SubscriptionId.Format(id);
        formatted.Should().StartWith("sub_");
    }
}
