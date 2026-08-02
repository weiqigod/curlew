using ApiTool.Backend.Results;

namespace ApiTool.Backend.Tests.Results;

/// <summary>Tests for the <see cref="ResultId"/> wire-format helper.</summary>
public sealed class ResultIdTests
{
    [Theory]
    [InlineData("res_00000000000000000000000000000000", true)]
    [InlineData("res_abcdef0123456789abcdef0123456789", true)]
    [InlineData("", false)]
    [InlineData("res_", false)]
    [InlineData("org_00000000000000000000000000000000", false)]
    [InlineData("res_xyz", false)]
    public void TryParse_accepts_valid_ids_and_rejects_invalid(string input, bool expected)
    {
        var result = ResultId.TryParse(input, out _);
        result.Should().Be(expected, because: $"'{input}' should be {(expected ? "valid" : "invalid")}");
    }

    [Fact]
    public void Format_then_TryParse_roundtrips()
    {
        var id = Guid.NewGuid();
        var formatted = ResultId.Format(id);
        var parsed = ResultId.TryParse(formatted, out var parsed_id);

        parsed.Should().BeTrue(because: "a freshly formatted id should always parse");
        parsed_id.Should().Be(id, because: "the roundtripped guid must be identical");
    }

    [Fact]
    public void Format_produces_res_prefix_followed_by_32_hex_chars()
    {
        var id = Guid.NewGuid();
        var formatted = ResultId.Format(id);

        formatted.Should().StartWith("res_");
        formatted[4..].Should().HaveLength(32);
        formatted[4..].Should().MatchRegex("^[0-9a-f]+$", because: "hex chars only, lowercase");
    }

    [Fact]
    public void TryParse_null_returns_false()
    {
        var result = ResultId.TryParse(null, out var id);
        result.Should().BeFalse();
        id.Should().Be(Guid.Empty);
    }
}
