namespace ApiTool.Backend.Tests.Coordinator;

/// <summary>Tests for <see cref="ApiTool.Backend.Coordinator.CoordinatorJobId"/>.</summary>
public sealed class CoordinatorJobIdTests
{
    [Theory]
    [InlineData("00000000-0000-0000-0000-000000000000", "job_00000000000000000000000000000000")]
    [InlineData("12345678-1234-1234-1234-123456789abc", "job_123456781234123412341234567 89abc")]
    public void Format_returns_prefixed_hex(string guidString, string expected)
    {
        var id = Guid.Parse(guidString);
        var result = ApiTool.Backend.Coordinator.CoordinatorJobId.Format(id);
        result.Should().Be(expected.Replace(" ", ""));
    }

    [Theory]
    [InlineData("job_00000000000000000000000000000000", true)]
    [InlineData("job_notahex", false)]
    [InlineData("res_00000000000000000000000000000000", false)]
    [InlineData(null, false)]
    [InlineData("", false)]
    public void TryParse_matches_expected(string? input, bool expected)
    {
        var result = ApiTool.Backend.Coordinator.CoordinatorJobId.TryParse(input, out var id);
        result.Should().Be(expected);
        if (!expected)
            id.Should().Be(Guid.Empty);
    }

    [Fact]
    public void Format_then_TryParse_roundtrips()
    {
        var original = Guid.NewGuid();
        var wire = ApiTool.Backend.Coordinator.CoordinatorJobId.Format(original);
        var ok = ApiTool.Backend.Coordinator.CoordinatorJobId.TryParse(wire, out var parsed);
        ok.Should().BeTrue();
        parsed.Should().Be(original);
    }
}
