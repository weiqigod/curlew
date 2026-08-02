using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Telemetry;

/// <summary>
/// Verifies that EF Core creates the telemetry_events and telemetry_daily_aggregates tables
/// with the required constraints (idempotency_key UNIQUE; composite PK on daily aggregates).
/// </summary>
public sealed class TelemetrySchemaTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;

    public TelemetrySchemaTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();

        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    [Fact]
    public async Task Telemetry_events_table_exists()
    {
        using var scope = OpenScope();
        var exists = await TableExistsAsync(scope.Connection, "telemetry_events");
        exists.Should().BeTrue(because: "M18-007 creates the telemetry_events table");
    }

    [Fact]
    public async Task Telemetry_daily_aggregates_table_exists()
    {
        using var scope = OpenScope();
        var exists = await TableExistsAsync(scope.Connection, "telemetry_daily_aggregates");
        exists.Should().BeTrue(because: "M18-007 creates the telemetry_daily_aggregates table");
    }

    [Fact]
    public async Task Telemetry_events_idempotency_key_is_unique()
    {
        using var scope = OpenScope();

        scope.Db.TelemetryEvents.Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = Guid.NewGuid(),
            EventType = "run.completed",
            EventPayloadJson = "{}",
            ReceivedAt = DateTime.UtcNow,
            IdempotencyKey = "dup-key-001",
        });
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        scope.Db.TelemetryEvents.Add(new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = Guid.NewGuid(),
            EventType = "run.completed",
            EventPayloadJson = "{}",
            ReceivedAt = DateTime.UtcNow,
            IdempotencyKey = "dup-key-001", // duplicate
        });

        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "idempotency_key must be UNIQUE on telemetry_events");
    }

    [Fact]
    public async Task Telemetry_daily_aggregates_composite_pk_day_event_type_is_unique()
    {
        using var scope = OpenScope();

        var day = new DateOnly(2026, 5, 1);
        scope.Db.TelemetryDailyAggregates.Add(new TelemetryDailyAggregate
        {
            Day = day,
            EventType = "run.completed",
            EventCount = 10,
            DistinctInstallCount = 3,
            NumericSumsJson = "{}",
        });
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        scope.Db.TelemetryDailyAggregates.Add(new TelemetryDailyAggregate
        {
            Day = day,
            EventType = "run.completed", // same composite PK
            EventCount = 5,
            DistinctInstallCount = 2,
            NumericSumsJson = "{}",
        });

        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "(day, event_type) is the composite PK of telemetry_daily_aggregates");
    }

    [Fact]
    public async Task Telemetry_events_has_expected_columns()
    {
        using var scope = OpenScope();
        var columns = await ListTableColumnsAsync(scope.Connection, "telemetry_events");
        columns.Should().Contain(["Id", "install_id", "event_type", "event_payload",
            "received_at", "idempotency_key"],
            because: "telemetry_events must have all required columns");
    }

    [Fact]
    public async Task Telemetry_daily_aggregates_has_expected_columns()
    {
        using var scope = OpenScope();
        var columns = await ListTableColumnsAsync(scope.Connection, "telemetry_daily_aggregates");
        columns.Should().Contain(["day", "event_type", "event_count",
            "distinct_install_count", "numeric_sums"],
            because: "telemetry_daily_aggregates must have all required columns");
    }

    private static async Task<bool> TableExistsAsync(SqliteConnection conn, string tableName)
    {
        await using var cmd = conn.CreateCommand();
        cmd.CommandText = "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=@name";
        var p = cmd.CreateParameter();
        p.ParameterName = "@name";
        p.Value = tableName;
        cmd.Parameters.Add(p);
        var result = await cmd.ExecuteScalarAsync();
        return Convert.ToInt64(result) > 0;
    }

    private static async Task<IReadOnlyList<string>> ListTableColumnsAsync(
        SqliteConnection conn, string tableName)
    {
        var columns = new List<string>();
        await using var cmd = conn.CreateCommand();
        cmd.CommandText = $"PRAGMA table_info({tableName})";
        await using var reader = await cmd.ExecuteReaderAsync();
        while (await reader.ReadAsync())
            columns.Add(reader.GetString(1));
        return columns;
    }
}
