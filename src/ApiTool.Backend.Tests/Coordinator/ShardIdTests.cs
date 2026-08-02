namespace ApiTool.Backend.Tests.Coordinator;

/// <summary>Tests for <see cref="ApiTool.Backend.Coordinator.ShardId"/>.</summary>
public sealed class ShardIdTests
{
    [Theory]
    [InlineData("00000000-0000-0000-0000-000000000000", "shd_00000000000000000000000000000000")]
    [InlineData("12345678-1234-1234-1234-123456789abc", "shd_123456781234123412341234567 89abc")]
    public void Format_returns_prefixed_hex(string guidString, string expected)
    {
        var id = Guid.Parse(guidString);
        var result = ApiTool.Backend.Coordinator.ShardId.Format(id);
        result.Should().Be(expected.Replace(" ", ""));
    }

    [Theory]
    [InlineData("shd_00000000000000000000000000000000", true)]
    [InlineData("shd_notahex", false)]
    [InlineData("job_00000000000000000000000000000000", false)]
    [InlineData(null, false)]
    [InlineData("", false)]
    public void TryParse_matches_expected(string? input, bool expected)
    {
        var result = ApiTool.Backend.Coordinator.ShardId.TryParse(input, out var id);
        result.Should().Be(expected);
        if (!expected)
            id.Should().Be(Guid.Empty);
    }

    [Fact]
    public void Format_then_TryParse_roundtrips()
    {
        var original = Guid.NewGuid();
        var wire = ApiTool.Backend.Coordinator.ShardId.Format(original);
        var ok = ApiTool.Backend.Coordinator.ShardId.TryParse(wire, out var parsed);
        ok.Should().BeTrue();
        parsed.Should().Be(original);
    }
}
