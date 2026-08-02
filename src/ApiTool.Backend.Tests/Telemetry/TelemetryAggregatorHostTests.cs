using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Telemetry;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using System.Text.Json;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Unit tests for <see cref="TelemetryAggregatorHost.TickOnceAsync"/>.
/// Uses in-memory SQLite for realistic EF Core bulk-query behaviour.
/// </summary>
public sealed class TelemetryAggregatorHostTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    // Fixed "now" at 2026-06-02T06:00:00Z — the previous two days are "closed" UTC days.
    private static readonly DateTimeOffset Now =
        new DateTimeOffset(2026, 6, 2, 6, 0, 0, TimeSpan.Zero);

    public TelemetryAggregatorHostTests()
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

    private TelemetryAggregatorHost BuildHost(TimeSpan? tickInterval = null)
    {
        var opts = Options.Create(new TelemetryAggregatorOptions());
        return new TelemetryAggregatorHost(
            BuildScopeFactory(_conn),
            opts,
            _clock,
            NullLogger<TelemetryAggregatorHost>.Instance,
            tickInterval ?? TimeSpan.FromMilliseconds(1));
    }

    private async Task SeedEventAsync(TestDbScope scope,
        Guid installId, string eventType, DateTime receivedAt, string payloadJson = "{}")
    {
        scope.Db.TelemetryEvents.Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = installId,
            EventType = eventType,
            EventPayloadJson = payloadJson,
            ReceivedAt = receivedAt,
            IdempotencyKey = Guid.NewGuid().ToString(),
        });
        await scope.Db.SaveChangesAsync();
    }

    [Fact]
    public async Task TickOnceAsync_produces_one_row_per_event_type_per_day()
    {
        var yesterday = Now.UtcDateTime.Date.AddDays(-1);
        var installA = Guid.NewGuid();
        var installB = Guid.NewGuid();

        using var scope = OpenScope();
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(1));
        await SeedEventAsync(scope, installB, "run.completed", yesterday.AddHours(2));
        await SeedEventAsync(scope, installA, "run.failed", yesterday.AddHours(3));

        var host = BuildHost();
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var rows = await verify.Db.TelemetryDailyAggregates.ToListAsync();

        rows.Should().HaveCount(2, because: "two distinct event types on one day → two aggregate rows");
        rows.Should().Contain(r => r.EventType == "run.completed" &&
                                   r.Day == DateOnly.FromDateTime(yesterday) &&
                                   r.EventCount == 2);
        rows.Should().Contain(r => r.EventType == "run.failed" &&
                                   r.Day == DateOnly.FromDateTime(yesterday) &&
                                   r.EventCount == 1);
    }

    [Fact]
    public async Task TickOnceAsync_counts_distinct_install_ids()
    {
        var yesterday = Now.UtcDateTime.Date.AddDays(-1);
        var installA = Guid.NewGuid();
        var installB = Guid.NewGuid();

        using var scope = OpenScope();
        // Three events from two distinct installs.
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(1));
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(2));
        await SeedEventAsync(scope, installB, "run.completed", yesterday.AddHours(3));

        var host = BuildHost();
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var row = await verify.Db.TelemetryDailyAggregates.SingleAsync(
            r => r.EventType == "run.completed");

        row.EventCount.Should().Be(3);
        row.DistinctInstallCount.Should().Be(2, because: "two distinct install_ids emitted run.completed");
    }

    [Fact]
    public async Task TickOnceAsync_aggregates_numeric_payload_fields_sum_min_max_avg()
    {
        var yesterday = Now.UtcDateTime.Date.AddDays(-1);
        var installA = Guid.NewGuid();

        using var scope = OpenScope();
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(1),
            """{"duration_ms": 100}""");
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(2),
            """{"duration_ms": 300}""");

        var host = BuildHost();
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var row = await verify.Db.TelemetryDailyAggregates.SingleAsync(
            r => r.EventType == "run.completed");

        using var jsonDoc = JsonDocument.Parse(row.NumericSumsJson);
        var durationMs = jsonDoc.RootElement.GetProperty("duration_ms");
        durationMs.GetProperty("sum").GetDouble().Should().Be(400);
        durationMs.GetProperty("min").GetDouble().Should().Be(100);
        durationMs.GetProperty("max").GetDouble().Should().Be(300);
        durationMs.GetProperty("avg").GetDouble().Should().Be(200);
        durationMs.GetProperty("n").GetInt32().Should().Be(2);
    }

    [Fact]
    public async Task TickOnceAsync_skips_days_already_aggregated()
    {
        var yesterday = Now.UtcDateTime.Date.AddDays(-1);
        var installA = Guid.NewGuid();

        using var scope = OpenScope();
        await SeedEventAsync(scope, installA, "run.completed", yesterday.AddHours(1));

        // Pre-seed an aggregate row for yesterday.
        scope.Db.TelemetryDailyAggregates.Add(new TelemetryDailyAggregate
        {
            Day = DateOnly.FromDateTime(yesterday),
            EventType = "run.completed",
            EventCount = 99,
            DistinctInstallCount = 5,
            NumericSumsJson = "{}",
        });
        await scope.Db.SaveChangesAsync();

        var host = BuildHost();
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var row = await verify.Db.TelemetryDailyAggregates.SingleAsync(
            r => r.EventType == "run.completed");

        // Should not have overwritten the existing row.
        row.EventCount.Should().Be(99, because: "already-aggregated days must be skipped");
    }

    [Fact]
    public async Task TickOnceAsync_skips_current_open_day()
    {
        // Seed events for today (the open day — should not be aggregated).
        var today = Now.UtcDateTime.Date;
        var installA = Guid.NewGuid();

        using var scope = OpenScope();
        await SeedEventAsync(scope, installA, "run.completed", today.AddHours(1));

        var host = BuildHost();
        await host.TickOnceAsync(CancellationToken.None);

        using var verify = OpenScope();
        var count = await verify.Db.TelemetryDailyAggregates.CountAsync();
        count.Should().Be(0, because: "the current open day must not be aggregated");
    }
}
