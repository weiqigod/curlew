// Refs docs/SPECIFICATION.md:8638-8642 (markdown safety + size truncation).
namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>Tests for <see cref="ApiTool.Backend.PrChecks.MarkdownSafety"/>.</summary>
public sealed class MarkdownSafetyTests
{
    [Fact]
    public void Truncate_returns_input_when_within_limit()
    {
        var input = new string('a', ApiTool.Backend.PrChecks.MarkdownSafety.MaxField);
        ApiTool.Backend.PrChecks.MarkdownSafety.Truncate(input)
            .Should().Be(input);
    }

    [Fact]
    public void Truncate_appends_truncated_marker_when_over_limit()
    {
        var input = new string('x', ApiTool.Backend.PrChecks.MarkdownSafety.MaxField + 1);
        var result = ApiTool.Backend.PrChecks.MarkdownSafety.Truncate(input);
        result.Length.Should().Be(ApiTool.Backend.PrChecks.MarkdownSafety.MaxField);
        result.Should().EndWith("(truncated)");
    }

    [Fact]
    public void Truncate_returns_empty_for_null()
    {
        ApiTool.Backend.PrChecks.MarkdownSafety.Truncate(null)
            .Should().Be(string.Empty);
    }

    [Fact]
    public void Truncate_returns_empty_for_empty()
    {
        ApiTool.Backend.PrChecks.MarkdownSafety.Truncate(string.Empty)
            .Should().Be(string.Empty);
    }

    [Theory]
    [InlineData("Hello *world*",   @"Hello \*world\*")]
    [InlineData("a_b_c",          @"a\_b\_c")]
    [InlineData("`code`",         @"\`code\`")]
    [InlineData("[link]",         @"\[link\]")]
    [InlineData("a\\b",           @"a\\b")]
    [InlineData("plain text",     "plain text")]
    public void EscapeUserContent_escapes_markdown_specials(string input, string expected)
    {
        ApiTool.Backend.PrChecks.MarkdownSafety.EscapeUserContent(input)
            .Should().Be(expected);
    }

    [Fact]
    public void EscapeUserContent_handles_empty_input()
    {
        ApiTool.Backend.PrChecks.MarkdownSafety.EscapeUserContent(string.Empty)
            .Should().Be(string.Empty);
    }
}
