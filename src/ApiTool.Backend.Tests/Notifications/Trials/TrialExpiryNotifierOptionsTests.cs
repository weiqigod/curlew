using ApiTool.Backend.Notifications.Trials;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Trials;

public sealed class TrialExpiryNotifierOptionsTests
{
    [Theory]
    [InlineData("09:00", 9, 0, 0)]
    [InlineData("09:00:30", 9, 0, 30)]
    [InlineData("00:00", 0, 0, 0)]
    [InlineData("23:59", 23, 59, 0)]
    public void ParseRunAtUtc_accepts_valid_time(string input, int h, int m, int s)
    {
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = input };
        opts.IsForcedFire.Should().BeFalse();
        opts.ParseRunAtUtc().Should().Be(new TimeOnly(h, m, s));
    }

    [Theory]
    [InlineData("now")]
    [InlineData("NOW")]
    [InlineData("Now")]
    public void IsForcedFire_true_for_now_sentinel(string sentinel)
    {
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = sentinel };
        opts.IsForcedFire.Should().BeTrue();
    }

    [Theory]
    [InlineData("9am")]
    [InlineData("noon")]
    [InlineData("25:00")]
    [InlineData("")]
    public void ParseRunAtUtc_throws_on_invalid_format(string bogus)
    {
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = bogus };
        FluentActions.Invoking(() => opts.ParseRunAtUtc())
            .Should().Throw<FormatException>();
    }

    [Fact]
    public void Default_section_key_matches_spec()
    {
        TrialExpiryNotifierOptions.Section.Should().Be("ApiTool:TrialExpiryNotifier");
    }

    [Fact]
    public void Default_RunAtUtc_is_0900()
    {
        new TrialExpiryNotifierOptions().RunAtUtc.Should().Be("09:00");
    }

    [Fact]
    public void Default_BatchSize_is_500()
    {
        new TrialExpiryNotifierOptions().BatchSize.Should().Be(500);
    }
}
