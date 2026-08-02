// Refs docs/SPECIFICATION.md:8643 (PRCHECK_TOKEN_LEAK_DETECTED defensive check).
namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>Tests for <see cref="ApiTool.Backend.PrChecks.TokenLeakDetector"/>.</summary>
public sealed class TokenLeakDetectorTests
{
    [Theory]
    [InlineData("""{"repo":"owner/repo","pr":1,"state":"success"}""", false)]
    [InlineData("ghs_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",           true)]   // exactly 36 A chars
    [InlineData("""{"token":"ghs_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}""", true)]  // 38 chars
    [InlineData("ghs_short",                                           false)]  // only 5 chars after ghs_
    [InlineData(null,                                                  false)]  // null input is safe
    [InlineData("",                                                    false)]  // empty input is safe
    [InlineData("ghs_" + "a",                                         false)]  // single char — too short
    public void ContainsTokenLeak_detects_ghs_tokens(string? body, bool expected)
    {
        ApiTool.Backend.PrChecks.TokenLeakDetector.ContainsTokenLeak(body)
            .Should().Be(expected);
    }

    [Fact]
    public void ContainsTokenLeak_detects_embedded_token_in_json_field()
    {
        const string body = """{"summary":"see ghs_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz for details"}""";
        ApiTool.Backend.PrChecks.TokenLeakDetector.ContainsTokenLeak(body)
            .Should().BeTrue();
    }

    [Fact]
    public void ContainsTokenLeak_returns_false_for_35_char_suffix()
    {
        // 35 alphanumeric chars after ghs_ — one short of the 36-char minimum
        var body = "ghs_" + new string('a', 35);
        ApiTool.Backend.PrChecks.TokenLeakDetector.ContainsTokenLeak(body)
            .Should().BeFalse();
    }

    [Fact]
    public void ContainsTokenLeak_returns_true_for_very_long_token()
    {
        var body = "ghs_" + new string('B', 100);
        ApiTool.Backend.PrChecks.TokenLeakDetector.ContainsTokenLeak(body)
            .Should().BeTrue();
    }
}
