// Tests for GitLabRateLimitTracker (Guid-keyed rate-limit tracker for GitLab).
// Refs M16-014 plan step 5.
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Unit tests for <see cref="GitLabRateLimitTracker"/> covering blocked/unblocked state,
/// retry-after backoff, reset-time backoff, and clear semantics.
/// Named so the filter FullyQualifiedName~GitLabRateLimitTracker matches.
/// </summary>
public sealed class GitLabRateLimitTrackerTests
{
    private static readonly DateTimeOffset Base = new(2026, 5, 11, 12, 0, 0, TimeSpan.Zero);

    [Fact]
    public void IsBlocked_NewInstallation_ReturnsFalse()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        tracker.IsBlocked(Guid.NewGuid()).Should().BeFalse();
    }

    [Fact]
    public void Update_WithRetryAfter_BlocksUntilRetryAfterElapses()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id = Guid.NewGuid();

        tracker.Update(id, remaining: null, retryAfter: 60, resetAt: null);

        tracker.IsBlocked(id).Should().BeTrue("blocked for 60 s");

        // Advance past the retry window
        clock.Advance(TimeSpan.FromSeconds(61));
        tracker.IsBlocked(id).Should().BeFalse("60 s elapsed");
    }

    [Fact]
    public void Update_WithRemainingZeroAndResetHeader_BlocksUntilReset()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id = Guid.NewGuid();
        var resetAt = Base.AddMinutes(5);

        tracker.Update(id, remaining: 0, retryAfter: null, resetAt: resetAt);

        tracker.IsBlocked(id).Should().BeTrue();

        // Just before reset
        clock.Advance(TimeSpan.FromMinutes(4).Add(TimeSpan.FromSeconds(59)));
        tracker.IsBlocked(id).Should().BeTrue();

        // After reset
        clock.Advance(TimeSpan.FromSeconds(2));
        tracker.IsBlocked(id).Should().BeFalse();
    }

    [Fact]
    public void Update_WithAboveWatermarkRemaining_DoesNotBlock()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id = Guid.NewGuid();

        tracker.Update(id, remaining: 200, retryAfter: null, resetAt: Base.AddMinutes(5));

        tracker.IsBlocked(id).Should().BeFalse("remaining is above low-watermark");
    }

    [Fact]
    public void IsBlocked_AfterResetTime_ReturnsFalse()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id = Guid.NewGuid();

        tracker.Update(id, remaining: 0, retryAfter: null, resetAt: Base.AddMinutes(1));
        clock.Advance(TimeSpan.FromMinutes(2));

        tracker.IsBlocked(id).Should().BeFalse();
    }

    [Fact]
    public void Clear_RemovesStateForInstallation()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id = Guid.NewGuid();

        tracker.Update(id, remaining: null, retryAfter: 120, resetAt: null);
        tracker.IsBlocked(id).Should().BeTrue();

        tracker.Clear(id);
        tracker.IsBlocked(id).Should().BeFalse();
    }

    [Fact]
    public void Update_TwoInstallations_TracksIndependently()
    {
        var clock = new FakeClock(Base);
        var tracker = new GitLabRateLimitTracker(clock);
        var id1 = Guid.NewGuid();
        var id2 = Guid.NewGuid();

        tracker.Update(id1, remaining: null, retryAfter: 60, resetAt: null);

        tracker.IsBlocked(id1).Should().BeTrue();
        tracker.IsBlocked(id2).Should().BeFalse();
    }
}
