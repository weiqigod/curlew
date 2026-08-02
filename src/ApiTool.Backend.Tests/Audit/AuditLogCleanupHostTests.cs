using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>
/// Unit tests for <see cref="AuditLogCleanupHost.TickOnceAsync"/>.
/// Uses an in-memory SQLite database for realistic EF Core bulk-delete behaviour.
/// </summary>
public sealed class AuditLogCleanupHostTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    // Fixed "now" at 2026-06-01T12:00:00Z
    private static readonly DateTimeOffset Now =
        new DateTimeOffset(2026, 6, 1, 12, 0, 0, TimeSpan.Zero);

    public AuditLogCleanupHostTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);

        // EnsureCreated on a shared connection so all tests share the schema.
        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private static IServiceScopeFactory BuildScopeFactory(SqliteConnection conn)
    {
        var services = new ServiceCollection();
        services.AddDbContext<AppDbContext>(opts => opts.UseSqlite(conn));
        return services.BuildServiceProvider().GetRequiredService<IServiceScopeFactory>();
    }

    private static AuditLogCleanupHost BuildHost(
        IServiceScopeFactory scopeFactory,
        TimeProvider clock,
        TimeSpan? tickInterval = null)
    {
        var opts = Options.Create(new AuditLogCleanupOptions());
        return new AuditLogCleanupHost(
            scopeFactory, opts, clock,
            NullLogger<AuditLogCleanupHost>.Instance,
            tickInterval ?? TimeSpan.FromMilliseconds(1));
    }

    private async Task<(Guid orgId, Guid ownerId)> SeedOrgAsync(
        TestDbScope scope, int retentionDays)
    {
        var ownerId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        scope.Db.Users.Add(new User
        {
            Id = ownerId,
            Email = $"cleanup-{ownerId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = $"Cleanup-{orgId:N}"[..30],
            Slug = $"cl-{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
            AuditLogRetentionDays = retentionDays,
        });
        await scope.Db.SaveChangesAsync();
        return (orgId, ownerId);
    }

    private async Task SeedAuditRowsAsync(TestDbScope scope, Guid orgId, Guid actorId,
        int countWithinWindow, int countExpired, int retentionDays)
    {
        var cutoff = Now.UtcDateTime.AddDays(-retentionDays);

        // Rows within the retention window (newer than cutoff)
        for (var i = 0; i < countWithinWindow; i++)
            scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                ActorId = actorId,
                EventType = "within.window",
                CreatedAt = cutoff.AddSeconds(i + 1),
                Success = true,
            });

        // Rows older than cutoff (should be deleted)
        for (var i = 0; i < countExpired; i++)
            scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                ActorId = actorId,
                EventType = "expired",
                CreatedAt = cutoff.AddDays(-1).AddSeconds(-i),
                Success = true,
            });

        await scope.Db.SaveChangesAsync();
    }

    [Fact]
    public async Task TickOnceAsync_deletes_rows_older_than_retention_for_each_org()
    {
        using var scope = OpenScope();
        var (orgId, ownerId) = await SeedOrgAsync(scope, retentionDays: 30);
        // 60 within window, 40 expired
        await SeedAuditRowsAsync(scope, orgId, ownerId,
            countWithinWindow: 60, countExpired: 40, retentionDays: 30);

        var host = BuildHost(BuildScopeFactory(_conn), _clock);
        await host.TickOnceAsync(CancellationToken.None);

        using var verifyScope = OpenScope();
        var remaining = await verifyScope.Db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgId);
        remaining.Should().Be(60);
    }

    [Fact]
    public async Task TickOnceAsync_honours_per_org_retention_independently()
    {
        using var scope = OpenScope();
        // Org A: retention=30 → 40 expired rows should be deleted
        var (orgAId, ownerAId) = await SeedOrgAsync(scope, retentionDays: 30);
        await SeedAuditRowsAsync(scope, orgAId, ownerAId,
            countWithinWindow: 60, countExpired: 40, retentionDays: 30);

        // Org B: retention=365 → same row ages, but nothing expired relative to 365-day window
        var (orgBId, ownerBId) = await SeedOrgAsync(scope, retentionDays: 365);
        // All 100 rows are "within window" for 365 days (they are only 1 day old)
        await SeedAuditRowsAsync(scope, orgBId, ownerBId,
            countWithinWindow: 100, countExpired: 0, retentionDays: 365);

        var host = BuildHost(BuildScopeFactory(_conn), _clock);
        await host.TickOnceAsync(CancellationToken.None);

        using var verifyScope = OpenScope();
        var remainingA = await verifyScope.Db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgAId);
        var remainingB = await verifyScope.Db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgBId);

        remainingA.Should().Be(60, "org A retention=30 should have 40 rows deleted");
        remainingB.Should().Be(100, "org B retention=365 should have no rows deleted");
    }

    [Fact]
    public async Task TickOnceAsync_keeps_rows_within_retention_window()
    {
        using var scope = OpenScope();
        var (orgId, ownerId) = await SeedOrgAsync(scope, retentionDays: 90);
        // All 50 rows are within 90-day window
        await SeedAuditRowsAsync(scope, orgId, ownerId,
            countWithinWindow: 50, countExpired: 0, retentionDays: 90);

        var host = BuildHost(BuildScopeFactory(_conn), _clock);
        await host.TickOnceAsync(CancellationToken.None);

        using var verifyScope = OpenScope();
        var remaining = await verifyScope.Db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgId);
        remaining.Should().Be(50, "no rows should be deleted when all are within the window");
    }

    [Fact]
    public async Task TickOnceAsync_logs_per_org_deletion_count()
    {
        using var scope = OpenScope();
        var (orgId, ownerId) = await SeedOrgAsync(scope, retentionDays: 30);
        await SeedAuditRowsAsync(scope, orgId, ownerId,
            countWithinWindow: 0, countExpired: 10, retentionDays: 30);

        var logger = new RecordingLogger<AuditLogCleanupHost>();
        var opts = Options.Create(new AuditLogCleanupOptions());
        var host = new AuditLogCleanupHost(
            BuildScopeFactory(_conn), opts, _clock, logger,
            TimeSpan.FromMilliseconds(1));

        await host.TickOnceAsync(CancellationToken.None);

        logger.Messages.Should().ContainSingle(m =>
            m.Contains("audit_log_cleanup") &&
            m.Contains(orgId.ToString()),
            because: "one log entry per org with deletion count should be emitted");
    }

    [Fact]
    public async Task TickOnceAsync_increments_prometheus_counter()
    {
        using var scope = OpenScope();
        var (orgId, ownerId) = await SeedOrgAsync(scope, retentionDays: 30);
        await SeedAuditRowsAsync(scope, orgId, ownerId,
            countWithinWindow: 0, countExpired: 5, retentionDays: 30);

        long totalMeasured = 0;
        using var meterListener = new System.Diagnostics.Metrics.MeterListener();
        meterListener.InstrumentPublished = (instrument, listener) =>
        {
            if (instrument.Meter.Name == "ApiTool.Backend.Audit" &&
                instrument.Name == "audit_log_cleanup_rows_deleted_total")
                listener.EnableMeasurementEvents(instrument);
        };
        meterListener.SetMeasurementEventCallback<long>((_, value, _, _) =>
            Interlocked.Add(ref totalMeasured, value));
        meterListener.Start();

        var host = BuildHost(BuildScopeFactory(_conn), _clock);
        await host.TickOnceAsync(CancellationToken.None);

        meterListener.RecordObservableInstruments();
        totalMeasured.Should().Be(5,
            because: "counter should be incremented by the number of deleted rows");
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }
}
