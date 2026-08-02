// Tests for GitLabStateMapper (state mapping + description building).
// Refs docs/SPECIFICATION.md:9233-9242 (lossy state-mapping table).
using ApiTool.Backend.PrChecks;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Tests for <see cref="GitLabStateMapper"/> covering all 6 CLI states, description markers,
/// and truncation behaviour. Named so the filter FullyQualifiedName~GitLabStateMapper matches.
/// </summary>
public sealed class GitLabStateMapperTests
{
    [Theory]
    [InlineData("success",   "success")]
    [InlineData("failure",   "failed")]
    [InlineData("cancelled", "canceled")]
    [InlineData("timed_out", "failed")]
    [InlineData("neutral",   "success")]
    [InlineData("skipped",   "success")]
    public void TryMap_AllSixCliStates_MapsToExpectedGitLabState(string cli, string expected)
    {
        GitLabStateMapper.TryMap(cli, out var actual).Should().BeTrue();
        actual.Should().Be(expected);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("action_required")]
    [InlineData("garbage")]
    public void TryMap_InvalidInput_ReturnsFalse(string? input)
    {
        GitLabStateMapper.TryMap(input, out _).Should().BeFalse();
    }

    [Fact]
    public void BuildDescription_TimedOut_PrependsMarker()
    {
        GitLabStateMapper.BuildDescription("timed_out", "12 passed, 1 failed")
            .Should().Be("[timed out] 12 passed, 1 failed");
    }

    [Fact]
    public void BuildDescription_Neutral_PrependsMarker()
    {
        GitLabStateMapper.BuildDescription("neutral", "no tests changed")
            .Should().Be("neutral: no tests changed");
    }

    [Fact]
    public void BuildDescription_Skipped_PrependsMarker()
    {
        GitLabStateMapper.BuildDescription("skipped", "no tests matched filter")
            .Should().Be("skipped: no tests matched filter");
    }

    [Fact]
    public void BuildDescription_LongerThan255Chars_TruncatesWithMarker()
    {
        var longText = new string('x', 300);
        var result = GitLabStateMapper.BuildDescription("success", longText);
        result.Length.Should().Be(GitLabStateMapper.MaxDescriptionLength);
        result.Should().EndWith("…(truncated)");
    }

    [Fact]
    public void BuildDescription_NullOriginalForLossyState_StillEmitsMarker()
    {
        GitLabStateMapper.BuildDescription("timed_out", null)
            .Should().Be("[timed out] ");
    }

    [Fact]
    public void BuildDescription_ExactlyAtLimit_NotTruncated()
    {
        var text = new string('y', GitLabStateMapper.MaxDescriptionLength);
        var result = GitLabStateMapper.BuildDescription("success", text);
        result.Length.Should().Be(GitLabStateMapper.MaxDescriptionLength);
        result.Should().NotEndWith("…(truncated)");
    }

    [Fact]
    public void BuildDescription_Success_NoPrefix()
    {
        GitLabStateMapper.BuildDescription("success", "all tests passed")
            .Should().Be("all tests passed");
    }

    [Fact]
    public void BuildDescription_Failure_NoPrefix()
    {
        GitLabStateMapper.BuildDescription("failure", "3 failed")
            .Should().Be("3 failed");
    }

    [Fact]
    public void BuildDescription_NullDescription_ReturnsEmptyForNonLossy()
    {
        GitLabStateMapper.BuildDescription("success", null)
            .Should().Be(string.Empty);
    }
}
