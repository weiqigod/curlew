using ApiTool.Backend.Results.Dashboard;

namespace ApiTool.Backend.Tests.Results.Dashboard;

/// <summary>Tests for <see cref="DashboardWindowExtensions"/>.</summary>
public sealed class DashboardWindowTests
{
    [Theory]
    [InlineData("7d",  DashboardWindow.SevenDays)]
    [InlineData("30d", DashboardWindow.ThirtyDays)]
    [InlineData("90d", DashboardWindow.NinetyDays)]
    [InlineData(null,  DashboardWindow.ThirtyDays)]   // default when omitted
    [InlineData("",    DashboardWindow.ThirtyDays)]   // default when empty
    public void TryParse_accepts_allowed_values(string? input, DashboardWindow expected)
    {
        var ok = DashboardWindowExtensions.TryParse(input, out var window);
        Assert.True(ok);
        Assert.Equal(expected, window);
    }

    [Theory]
    [InlineData("14d")]
    [InlineData("1d")]
    [InlineData("7D")]      // case-sensitive
    [InlineData(" 7d")]     // no whitespace trimming
    [InlineData("seven")]
    [InlineData("0d")]
    public void TryParse_rejects_unsupported_values(string input)
    {
        var ok = DashboardWindowExtensions.TryParse(input, out _);
        Assert.False(ok);
    }

    [Fact]
    public void ToTimeSpan_returns_correct_span()
    {
        Assert.Equal(TimeSpan.FromDays(7), DashboardWindow.SevenDays.ToTimeSpan());
        Assert.Equal(TimeSpan.FromDays(30), DashboardWindow.ThirtyDays.ToTimeSpan());
        Assert.Equal(TimeSpan.FromDays(90), DashboardWindow.NinetyDays.ToTimeSpan());
    }

    [Fact]
    public void ToWire_returns_correct_string()
    {
        Assert.Equal("7d", DashboardWindow.SevenDays.ToWire());
        Assert.Equal("30d", DashboardWindow.ThirtyDays.ToWire());
        Assert.Equal("90d", DashboardWindow.NinetyDays.ToWire());
    }

    [Fact]
    public void Default_is_ThirtyDays()
    {
        Assert.Equal(DashboardWindow.ThirtyDays, DashboardWindowExtensions.Default);
    }

    [Fact]
    public void AllowedValues_contains_three_entries()
    {
        Assert.Equal(3, DashboardWindowExtensions.AllowedValues.Count);
        Assert.Contains("7d", DashboardWindowExtensions.AllowedValues);
        Assert.Contains("30d", DashboardWindowExtensions.AllowedValues);
        Assert.Contains("90d", DashboardWindowExtensions.AllowedValues);
    }
}
