using ApiTool.Backend.Schedules;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Tests for <see cref="RunId"/> wire-format id helpers.</summary>
public sealed class RunIdTests
{
    [Fact]
    public void Format_returns_prefixed_hex()
    {
        var id = Guid.NewGuid();
        var formatted = RunId.Format(id);
        formatted.Should().StartWith("run_");
        formatted.Should().Be($"run_{id:N}");
    }

    [Fact]
    public void TryParse_round_trips()
    {
        var original = Guid.NewGuid();
        var formatted = RunId.Format(original);
        var parsed = RunId.TryParse(formatted, out var result);
        parsed.Should().BeTrue();
        result.Should().Be(original);
    }

    [Fact]
    public void TryParse_returns_false_for_wrong_prefix()
    {
        var id = Guid.NewGuid();
        var wrongPrefix = $"sched_{id:N}";
        var parsed = RunId.TryParse(wrongPrefix, out var result);
        parsed.Should().BeFalse();
        result.Should().Be(Guid.Empty);
    }

    [Fact]
    public void TryParse_returns_false_for_null()
    {
        var parsed = RunId.TryParse(null, out var result);
        parsed.Should().BeFalse();
        result.Should().Be(Guid.Empty);
    }
}
