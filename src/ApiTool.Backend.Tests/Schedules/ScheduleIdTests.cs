using ApiTool.Backend.Schedules;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Tests for <see cref="ScheduleId"/> wire-format id helpers.</summary>
public sealed class ScheduleIdTests
{
    [Fact]
    public void Format_returns_prefixed_hex()
    {
        var id = Guid.NewGuid();
        var formatted = ScheduleId.Format(id);
        formatted.Should().StartWith("sched_");
        formatted.Should().Be($"sched_{id:N}");
    }

    [Fact]
    public void TryParse_round_trips()
    {
        var original = Guid.NewGuid();
        var formatted = ScheduleId.Format(original);
        var parsed = ScheduleId.TryParse(formatted, out var result);
        parsed.Should().BeTrue();
        result.Should().Be(original);
    }

    [Fact]
    public void TryParse_returns_false_for_wrong_prefix()
    {
        var id = Guid.NewGuid();
        var wrongPrefix = $"run_{id:N}";
        var parsed = ScheduleId.TryParse(wrongPrefix, out var result);
        parsed.Should().BeFalse();
        result.Should().Be(Guid.Empty);
    }

    [Fact]
    public void TryParse_returns_false_for_null()
    {
        var parsed = ScheduleId.TryParse(null, out var result);
        parsed.Should().BeFalse();
        result.Should().Be(Guid.Empty);
    }
}
