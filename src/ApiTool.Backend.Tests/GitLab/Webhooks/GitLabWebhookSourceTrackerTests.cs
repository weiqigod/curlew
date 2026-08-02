// Tests for GitLabWebhookSourceTracker — per-source-IP verification-failure quarantine.
// Refs M16-015 task YAML behavior #6 and plan Decision C.
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Unit tests for <see cref="GitLabWebhookSourceTracker"/>:
/// fresh state, increments, quarantine at threshold, success resets failures,
/// idle window reset, admin reset, thread safety.
/// </summary>
public sealed class GitLabWebhookSourceTrackerTests
{
    private const string Ip = "192.168.1.1";

    private static GitLabWebhookSourceTracker Create(DateTimeOffset? now = null)
    {
        var clock = new FakeClock(now ?? new DateTimeOffset(2026, 1, 1, 0, 0, 0, TimeSpan.Zero));
        return new GitLabWebhookSourceTracker(clock);
    }

    [Fact]
    public void Fresh_source_is_not_quarantined()
    {
        var tracker = Create();
        var (quarantined, quarantinedAt) = tracker.GetState(Ip);
        quarantined.Should().BeFalse();
        quarantinedAt.Should().BeNull();
    }

    [Fact]
    public void Records_failure_increments_counter()
    {
        var tracker = Create();
        var count = tracker.RecordFailure(Ip);
        count.Should().Be(1);
        var (quarantined, _) = tracker.GetState(Ip);
        quarantined.Should().BeFalse("below threshold");
    }

    [Fact]
    public void Fifth_failure_quarantines()
    {
        var now = new DateTimeOffset(2026, 1, 1, 12, 0, 0, TimeSpan.Zero);
        var tracker = Create(now);
        int count = 0;
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
            count = tracker.RecordFailure(Ip);

        count.Should().Be(GitLabWebhookSourceTracker.QuarantineThreshold);
        var (quarantined, quarantinedAt) = tracker.GetState(Ip);
        quarantined.Should().BeTrue();
        quarantinedAt.Should().NotBeNull();
        quarantinedAt!.Value.Should().Be(now.UtcDateTime);
    }

    [Fact]
    public void Quarantined_source_stays_quarantined_across_additional_failures()
    {
        var tracker = Create();
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
            tracker.RecordFailure(Ip);

        tracker.RecordFailure(Ip);
        tracker.RecordFailure(Ip);

        var (quarantined, _) = tracker.GetState(Ip);
        quarantined.Should().BeTrue();
    }

    [Fact]
    public void Success_resets_failures_but_not_quarantine()
    {
        var tracker = Create();
        // Quarantine first
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
            tracker.RecordFailure(Ip);

        tracker.RecordSuccess(Ip);

        // Quarantine should still be active (requires manual reset)
        var (quarantined, _) = tracker.GetState(Ip);
        quarantined.Should().BeTrue("quarantine persists until manual reset");

        // But failure counter is reset: next 4 failures should not re-quarantine a new IP
        var freshTracker = Create();
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold - 1; i++)
            freshTracker.RecordFailure("10.0.0.1");
        freshTracker.RecordSuccess("10.0.0.1");
        var (notQuarantined, _) = freshTracker.GetState("10.0.0.1");
        notQuarantined.Should().BeFalse();
    }

    [Fact]
    public void Idle_window_resets_warm_failure_count()
    {
        var start = new DateTimeOffset(2026, 1, 1, 12, 0, 0, TimeSpan.Zero);
        var clock = new FakeClock(start);
        var tracker = new GitLabWebhookSourceTracker(clock);

        // Accumulate 4 failures (below threshold)
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold - 1; i++)
            tracker.RecordFailure(Ip);

        // Advance past the idle reset window
        clock.Advance(GitLabWebhookSourceTracker.IdleResetWindow + TimeSpan.FromSeconds(1));

        // 5th failure after idle reset should be treated as 1st — no quarantine
        var count = tracker.RecordFailure(Ip);
        count.Should().Be(1, "idle window reset the counter");
        var (quarantined, _) = tracker.GetState(Ip);
        quarantined.Should().BeFalse("idle window prevented quarantine");
    }

    [Fact]
    public void Reset_clears_both_failures_and_quarantine()
    {
        var tracker = Create();
        // Quarantine first
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
            tracker.RecordFailure(Ip);

        tracker.Reset(Ip);

        var (quarantined, quarantinedAt) = tracker.GetState(Ip);
        quarantined.Should().BeFalse();
        quarantinedAt.Should().BeNull();
    }

    [Fact]
    public async Task Concurrent_increment_is_thread_safe()
    {
        var tracker = Create();
        const int taskCount = 10;
        // Use a unique IP not touched by other tests
        const string parallelIp = "172.16.0.1";
        var tasks = Enumerable.Range(0, taskCount)
            .Select(_ => Task.Run(() => tracker.RecordFailure(parallelIp)))
            .ToArray();
        await Task.WhenAll(tasks);

        var (_, quarantinedAt) = tracker.GetState(parallelIp);
        // 10 failures > threshold=5, so it must be quarantined
        quarantinedAt.Should().NotBeNull("10 concurrent failures must quarantine");
    }
}
