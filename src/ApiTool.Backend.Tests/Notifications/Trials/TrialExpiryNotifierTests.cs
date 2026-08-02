using ApiTool.Backend;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Notifications.Trials;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using FluentAssertions;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Diagnostics;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Notifications.Trials;

public sealed class TrialExpiryNotifierTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly FakeClock _clock;
    private readonly RecordingEmailQueue _queue;

    public TrialExpiryNotifierTests()
    {
        _scope = TestDb.CreateOpen();
        _clock = new FakeClock(new DateTimeOffset(2026, 5, 11, 12, 0, 0, TimeSpan.Zero));
        _queue = new RecordingEmailQueue();
    }

    public async ValueTask DisposeAsync() => await _scope.DisposeAsync();

    private TrialExpiryNotifier BuildSut(
        string runAtUtc = "now",
        TimeSpan? tickInterval = null)
    {
        var services = new ServiceCollection();
        var conn = _scope.Connection;
        services.AddDbContext<ApiTool.Backend.Data.AppDbContext>(o => o.UseSqlite(conn));
        services.AddSingleton<IEmailQueue>(_queue);
        var sp = services.BuildServiceProvider();

        return new TrialExpiryNotifier(
            sp.GetRequiredService<IServiceScopeFactory>(),
            Options.Create(new TrialExpiryNotifierOptions { RunAtUtc = runAtUtc, BatchSize = 100 }),
            Options.Create(new AppOptions { WebAppUrl = "https://app.apitool.dev" }),
            _clock,
            NullLogger<TrialExpiryNotifier>.Instance,
            tickInterval: tickInterval ?? TimeSpan.FromMilliseconds(1));
    }

    private async Task EnsureSchemaAsync() => await _scope.Db.Database.MigrateAsync();

    private async Task<Guid> SeedUserAsync(string? email = null)
    {
        var id = Guid.NewGuid();
        _scope.Db.Users.Add(new User
        {
            Id = id,
            Email = email ?? $"user-{id:N}@example.com",
            CreatedAt = _clock.GetUtcNow().UtcDateTime,
        });
        await _scope.Db.SaveChangesAsync();
        return id;
    }

    private async Task<Trial> SeedTrialAsync(
        Guid userId, string feature, TrialKind kind, DateTime expiresAt,
        DateTime? notified3day = null, DateTime? notified1day = null,
        DateTime? consumedAt = null)
    {
        var now = _clock.GetUtcNow().UtcDateTime;
        var t = new Trial
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            Feature = feature,
            Kind = kind,
            GrantedAt = now.AddDays(-1),
            ExpiresAt = expiresAt,
            ConsumedAt = consumedAt,
            Notified3DayAt = notified3day,
            Notified1DayAt = notified1day,
            CreatedAt = now,
            UpdatedAt = now,
        };
        _scope.Db.Trials.Add(t);
        await _scope.Db.SaveChangesAsync();
        return t;
    }

    [Fact]
    public async Task Tick_queues_3day_email_and_sets_notified_3day_at()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(3).AddHours(-1); // ~3d out
        var trial = await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Should().ContainSingle(m =>
            m.To == "alex@example.com" &&
            m.TemplateSlug == "trial_expiring" &&
            m.Variables["feature"] == "schedules" &&
            m.Variables["user_email"] == "alex@example.com" &&
            m.Variables["upgrade_url"] == "https://app.apitool.dev/billing");

        _scope.Db.ChangeTracker.Clear();
        var refreshed = _scope.Db.Trials.Single(t => t.Id == trial.Id);
        refreshed.Notified3DayAt.Should().NotBeNull();
        refreshed.Notified1DayAt.Should().BeNull();
    }

    [Fact]
    public async Task Tick_queues_1day_email_and_sets_notified_1day_at()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddHours(20); // ~1d out
        var trial = await SeedTrialAsync(userId, "dashboards", TrialKind.OnDemand, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        // Both 3-day and 1-day passes should match (window is 0–3d) — but the row
        // gets one message per pass. After this tick we expect TWO messages and BOTH
        // notified_*_at columns set, because the row falls in both windows.
        _queue.Messages.Should().HaveCount(2);
        _queue.Messages.Should().OnlyContain(m =>
            m.TemplateSlug == "trial_expiring" &&
            m.Variables["feature"] == "dashboards");

        _scope.Db.ChangeTracker.Clear();
        var refreshed = _scope.Db.Trials.Single(t => t.Id == trial.Id);
        refreshed.Notified3DayAt.Should().NotBeNull();
        refreshed.Notified1DayAt.Should().NotBeNull();
    }

    [Fact]
    public async Task Tick_does_not_send_duplicate_when_notified_3day_at_already_set()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync();
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        var alreadyNotified = _clock.GetUtcNow().UtcDateTime.AddHours(-1);
        var trial = await SeedTrialAsync(
            userId, "schedules", TrialKind.FullInitial, expires,
            notified3day: alreadyNotified);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        // 3-day already sent, but the 1-day window matches (2 days <= 3 days, NOT <= 1 day).
        // So zero emails from the 1-day pass.
        _queue.Messages.Should().BeEmpty();

        _scope.Db.ChangeTracker.Clear();
        var refreshed = _scope.Db.Trials.Single(t => t.Id == trial.Id);
        refreshed.Notified3DayAt.Should().Be(alreadyNotified, "timestamp must not be overwritten");
    }

    [Fact]
    public async Task Tick_skips_rows_with_preempted_by_subscription_kind()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync();
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        var trial = await SeedTrialAsync(
            userId, "schedules", TrialKind.PreemptedBySubscription, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Should().BeEmpty();
        _scope.Db.ChangeTracker.Clear();
        _scope.Db.Trials.Single(t => t.Id == trial.Id).Notified3DayAt.Should().BeNull();
    }

    [Fact]
    public async Task Tick_skips_rows_with_consumed_at_set()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync();
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        var trial = await SeedTrialAsync(
            userId, "schedules", TrialKind.FullInitial, expires,
            consumedAt: _clock.GetUtcNow().UtcDateTime.AddHours(-1));

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task Tick_skips_rows_already_expired()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync();
        var expires = _clock.GetUtcNow().UtcDateTime.AddHours(-1); // already past
        var trial = await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task Tick_skips_rows_expiring_beyond_3day_window()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync();
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(7); // far future
        var trial = await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task Tick_emits_message_with_manifest_allowlisted_variables_only()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        // Per spec :8587 — exactly these four keys.
        var expected = new[] { "user_email", "feature", "expires_at_local", "upgrade_url" };
        _queue.Messages[0].Variables.Keys.Should().BeEquivalentTo(expected);

        // Assert the expires_at_local value is formatted as "yyyy-MM-dd HH:mm UTC" (spec :8587).
        _queue.Messages[0].Variables["expires_at_local"]
            .Should().MatchRegex(@"^\d{4}-\d{2}-\d{2} \d{2}:\d{2} UTC$",
                "expires_at_local must use the yyyy-MM-dd HH:mm UTC format");
    }

    [Fact]
    public async Task Tick_processes_both_full_initial_and_ondemand_kinds()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("a@example.com");
        var userId2 = await SeedUserAsync("b@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);
        await SeedTrialAsync(userId2, "dashboards", TrialKind.OnDemand, expires);

        var sut = BuildSut();
        await sut.TickOnceAsync(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _queue.Messages.Select(m => m.To).Should().BeEquivalentTo(new[] { "a@example.com", "b@example.com" });
    }

    [Fact]
    public async Task TickOnceAsync_is_idempotent_across_two_immediate_invocations()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut();
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 };
        await sut.TickOnceAsync(opts, default);
        await sut.TickOnceAsync(opts, default);

        _queue.Messages.Should().HaveCount(1, "second tick must not re-enqueue");
    }

    [Fact]
    public async Task ExecuteAsync_with_now_sentinel_fires_at_least_once()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        var sut = BuildSut(runAtUtc: "now", tickInterval: TimeSpan.FromMilliseconds(1));
        using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));
        _ = sut.StartAsync(cts.Token);

        // Poll until the row's notified_3day_at is set, then stop the service.
        var deadline = DateTime.UtcNow.AddSeconds(5);
        while (DateTime.UtcNow < deadline)
        {
            _scope.Db.ChangeTracker.Clear();
            if (_scope.Db.Trials.Single().Notified3DayAt is not null)
                break;
            await Task.Delay(20);
        }
        await cts.CancelAsync();

        _scope.Db.ChangeTracker.Clear();
        _scope.Db.Trials.Single().Notified3DayAt.Should().NotBeNull();
        _queue.Messages.Should().NotBeEmpty();
    }

    [Fact]
    public async Task Tick_does_not_set_notified_at_when_enqueue_throws()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        var trial = await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        // Hand-rolled throwing queue. We bypass BuildSut because we need a different IEmailQueue.
        var throwingQueue = new ThrowingEmailQueue();
        var services = new ServiceCollection();
        services.AddDbContext<ApiTool.Backend.Data.AppDbContext>(o => o.UseSqlite(_scope.Connection));
        services.AddSingleton<IEmailQueue>(throwingQueue);
        var sp = services.BuildServiceProvider();

        var sut = new TrialExpiryNotifier(
            sp.GetRequiredService<IServiceScopeFactory>(),
            Options.Create(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }),
            Options.Create(new AppOptions { WebAppUrl = "https://app.apitool.dev" }),
            _clock,
            NullLogger<TrialExpiryNotifier>.Instance,
            tickInterval: TimeSpan.FromMilliseconds(1));

        await sut.TickOnceAsync(
            new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        _scope.Db.ChangeTracker.Clear();
        _scope.Db.Trials.Single(t => t.Id == trial.Id).Notified3DayAt.Should().BeNull(
            "row must remain unmodified when enqueue throws so the next tick can retry");
    }

    // ── Finding #1: ComputeNextFireDelay wall-clock scheduling accuracy ──────────

    [Fact]
    public void ComputeNextFireDelay_schedules_next_day_when_exactly_at_fire_time()
    {
        // Clock is exactly at the configured fire time — next fire is tomorrow.
        var now = new DateTimeOffset(2026, 5, 11, 9, 0, 0, TimeSpan.Zero);
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = "09:00" };

        var delay = TrialExpiryNotifier.ComputeNextFireDelay(opts, now);

        // At the exact fire time, todayFire == now so `now < todayFire` is false →
        // next = todayFire.AddDays(1), delay ≈ 24h.
        delay.Should().BeCloseTo(TimeSpan.FromHours(24), precision: TimeSpan.FromSeconds(1));
    }

    [Fact]
    public void ComputeNextFireDelay_fires_today_when_before_fire_time()
    {
        // Clock is 30 minutes before the 09:00 fire window.
        var now = new DateTimeOffset(2026, 5, 11, 8, 30, 0, TimeSpan.Zero);
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = "09:00" };

        var delay = TrialExpiryNotifier.ComputeNextFireDelay(opts, now);

        delay.Should().BeCloseTo(TimeSpan.FromMinutes(30), precision: TimeSpan.FromSeconds(1));
        delay.Should().BeLessThan(TimeSpan.FromHours(1));
    }

    [Fact]
    public void ComputeNextFireDelay_schedules_next_day_when_past_fire_time()
    {
        // Clock is 2 hours past the 09:00 fire window.
        var now = new DateTimeOffset(2026, 5, 11, 11, 0, 0, TimeSpan.Zero);
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = "09:00" };

        var delay = TrialExpiryNotifier.ComputeNextFireDelay(opts, now);

        // Next fire is tomorrow at 09:00 — 22 hours from now.
        delay.Should().BeCloseTo(TimeSpan.FromHours(22), precision: TimeSpan.FromSeconds(1));
        delay.TotalHours.Should().BeGreaterThan(20).And.BeLessThan(24);
    }

    [Fact]
    public void ComputeNextFireDelay_fuzz_tolerance_within_60_seconds_spec()
    {
        // Simulate clock at 08:59:01 (59 seconds before 09:00).
        // The delay returned must be <= 60 seconds, satisfying the spec's fuzz tolerance.
        var now = new DateTimeOffset(2026, 5, 11, 8, 59, 1, TimeSpan.Zero);
        var opts = new TrialExpiryNotifierOptions { RunAtUtc = "09:00" };

        var delay = TrialExpiryNotifier.ComputeNextFireDelay(opts, now);

        delay.Should().BeLessThanOrEqualTo(TimeSpan.FromSeconds(60),
            "spec requires the cron fires within 60s of the configured UTC time");
        delay.Should().BePositive("delay must be positive to avoid spinning");
    }

    // ── Finding #2: DbUpdateException branch ─────────────────────────────────────

    [Fact]
    public async Task Tick_does_not_set_notified_at_when_save_throws_DbUpdateException()
    {
        await EnsureSchemaAsync();
        var userId = await SeedUserAsync("alex@example.com");
        var expires = _clock.GetUtcNow().UtcDateTime.AddDays(2);
        var trial = await SeedTrialAsync(userId, "schedules", TrialKind.FullInitial, expires);

        // Register AppDbContext with a SaveChangesInterceptor that throws DbUpdateException.
        // This is the same pattern used in Bootstrap/TestDoubles.cs for rollback tests.
        var services = new ServiceCollection();
        services.AddDbContext<ApiTool.Backend.Data.AppDbContext>(o =>
            o.UseSqlite(_scope.Connection)
             .AddInterceptors(new DbUpdateExceptionInterceptor()));
        services.AddSingleton<IEmailQueue>(_queue);
        var sp = services.BuildServiceProvider();

        var sut = new TrialExpiryNotifier(
            sp.GetRequiredService<IServiceScopeFactory>(),
            Options.Create(new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }),
            Options.Create(new AppOptions { WebAppUrl = "https://app.apitool.dev" }),
            _clock,
            NullLogger<TrialExpiryNotifier>.Instance,
            tickInterval: TimeSpan.FromMilliseconds(1));

        await sut.TickOnceAsync(
            new TrialExpiryNotifierOptions { RunAtUtc = "now", BatchSize = 100 }, default);

        // Email was enqueued (queue is not throwing) ...
        _queue.Messages.Should().NotBeEmpty("the enqueue step precedes SaveChangesAsync");

        // ... but the notified_*_at column must remain null because SaveChangesAsync threw.
        _scope.Db.ChangeTracker.Clear();
        _scope.Db.Trials.Single(t => t.Id == trial.Id).Notified3DayAt.Should().BeNull(
            "row must remain unmodified when SaveChangesAsync throws so the next tick can retry");
    }

    /// <summary>
    /// EF Core save-changes interceptor that throws <see cref="DbUpdateException"/> on every
    /// <c>SavingChangesAsync</c> call. Used to exercise the DbUpdateException catch
    /// branch in <see cref="TrialExpiryNotifier"/>.
    /// </summary>
    private sealed class DbUpdateExceptionInterceptor : SaveChangesInterceptor
    {
        public override ValueTask<InterceptionResult<int>> SavingChangesAsync(
            DbContextEventData eventData,
            InterceptionResult<int> result,
            CancellationToken cancellationToken = default)
            => throw new DbUpdateException("simulated save failure",
                new InvalidOperationException("inner"));
    }

    private sealed class ThrowingEmailQueue : IEmailQueue
    {
        public ValueTask EnqueueAsync(EmailMessage message, CancellationToken ct = default)
            => throw new InvalidOperationException("simulated queue failure");
    }
}
