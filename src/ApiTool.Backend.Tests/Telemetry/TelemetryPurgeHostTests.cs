using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Telemetry;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Unit tests for <see cref="TelemetryPurgeHost.TickOnceAsync"/>.
/// Uses in-memory SQLite for realistic EF Core bulk-delete behaviour.
/// </summary>
public sealed class TelemetryPurgeHostTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    // Fixed "now" at 2026-06-01T12:00:00Z
    private static readonly DateTimeOffset Now =
        new DateTimeOffset(2026, 6, 1, 12, 0, 0, TimeSpan.Zero);

    public TelemetryPurgeHostTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);

        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private static IServiceScopeFactory BuildScopeFactory(SqliteConnection conn)
    {
        var services = new ServiceCollection();
        services.AddDbContext<AppDbContext>(opts => opts.UseSqlite(conn));
        return services.BuildServiceProvider().GetRequiredService<IServiceScopeFactory>();
    }

    private TelemetryPurgeHost BuildHost(int retentionDays = 90)
    {
        var opts = Options.Create(new TelemetryPurgeOptions { RetentionDays = retentionDays });
        return new TelemetryPurgeHost(
            BuildScopeFactory(_conn), opts, _clock,
            NullLogger<TelemetryPurgeHost>.Instance,
            tickInterval: TimeSpan.FromMilliseconds(1));
    }

    private async Task SeedEventAsync(TestDbScope scope, DateTime receivedAt)
    {
        scope.Db.TelemetryEvents.Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = Guid.NewGuid(),
            EventType = "run.completed",
            EventPayloadJson = "{}",
            ReceivedAt = receivedAt,
            IdempotencyKey = Guid.NewGuid().ToString(),
        });
        await scope.Db.SaveChangesAsync();
    }

    [Fact]
    public async Task TickOnceAsync_hard_deletes_events_older_than_90_days()
    {
        var oldDate = Now.UtcDateTime.AddDays(-91);
        var newDate = Now.UtcDateTime.AddDays(-10);

        using var scope = OpenScope();
        await SeedEventAsync(scope, oldDate);  // should be deleted
        await SeedEventAsync(scope, newDate);  // should be kept

        var host = BuildHost(retentionDays: 90);
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var remaining = await verify.Db.TelemetryEvents.ToListAsync();
        remaining.Should().HaveCount(1, because: "only the event within the window should remain");
        remaining[0].ReceivedAt.Should().BeCloseTo(newDate, TimeSpan.FromSeconds(1));
    }

    [Fact]
    public async Task TickOnceAsync_preserves_events_within_window()
    {
        // All events are recent — nothing should be deleted.
        using var scope = OpenScope();
        await SeedEventAsync(scope, Now.UtcDateTime.AddDays(-5));
        await SeedEventAsync(scope, Now.UtcDateTime.AddDays(-10));
        await SeedEventAsync(scope, Now.UtcDateTime.AddDays(-89));  // still within 90-day window

        var host = BuildHost(retentionDays: 90);
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var count = await verify.Db.TelemetryEvents.CountAsync();
        count.Should().Be(3, because: "events within the retention window must not be deleted");
    }

    [Fact]
    public async Task TickOnceAsync_does_not_touch_daily_aggregates()
    {
        // Pre-seed an aggregate row that is "old" (older than 90 days by date label).
        using var scope = OpenScope();
        scope.Db.TelemetryDailyAggregates.Add(new TelemetryDailyAggregate
        {
            Day = DateOnly.FromDateTime(Now.UtcDateTime.AddDays(-200)),
            EventType = "run.completed",
            EventCount = 42,
            DistinctInstallCount = 5,
            NumericSumsJson = "{}",
        });
        await scope.Db.SaveChangesAsync();

        var host = BuildHost(retentionDays: 90);
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var aggCount = await verify.Db.TelemetryDailyAggregates.CountAsync();
        aggCount.Should().Be(1, because: "TelemetryPurgeHost must not delete daily aggregate rows");
    }
}
