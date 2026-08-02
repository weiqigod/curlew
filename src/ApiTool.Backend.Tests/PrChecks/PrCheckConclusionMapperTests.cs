// Refs docs/SPECIFICATION.md:8501-8511 (6-state CLI→GitHub conclusion mapping).
namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>Table-driven tests for <see cref="ApiTool.Backend.PrChecks.PrCheckConclusionMapper"/>.</summary>
public sealed class PrCheckConclusionMapperTests
{
    [Theory]
    [InlineData("success",          true,  "success")]
    [InlineData("failure",          true,  "failure")]
    [InlineData("cancelled",        true,  "cancelled")]
    [InlineData("timed_out",        true,  "timed_out")]
    [InlineData("neutral",          true,  "neutral")]
    [InlineData("skipped",          true,  "skipped")]
    [InlineData("SUCCESS",          true,  "success")]    // case-insensitive input
    [InlineData("Failure",          true,  "failure")]    // mixed case
    [InlineData("action_required",  false, "")]           // banned in M14 per :8512
    [InlineData("",                 false, "")]
    [InlineData(null,               false, "")]
    [InlineData("pending",          false, "")]           // not a valid conclusion
    [InlineData("unknown_state",    false, "")]
    public void TryMap_maps_cli_state_to_github_conclusion(
        string? input, bool expectedOk, string expectedConclusion)
    {
        var ok = ApiTool.Backend.PrChecks.PrCheckConclusionMapper.TryMap(input, out var conclusion);
        ok.Should().Be(expectedOk);
        conclusion.Should().Be(expectedConclusion);
    }

    [Fact]
    public void ValidStates_contains_all_six_states()
    {
        var states = ApiTool.Backend.PrChecks.PrCheckConclusionMapper.ValidStates;
        states.Should().HaveCount(6);
        states.Should().Contain("success");
        states.Should().Contain("failure");
        states.Should().Contain("cancelled");
        states.Should().Contain("timed_out");
        states.Should().Contain("neutral");
        states.Should().Contain("skipped");
    }

    [Fact]
    public void ValidStates_does_not_contain_action_required()
    {
        // action_required is NEVER emitted in M14 per spec :8512
        ApiTool.Backend.PrChecks.PrCheckConclusionMapper.ValidStates
            .Should().NotContain("action_required");
    }
}
