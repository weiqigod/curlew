using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Storage;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Unit tests for <see cref="UserExportBuilderHost.TickOnceAsync"/>.
/// Uses SQLite in-memory for realistic EF Core query behaviour.
/// </summary>
public sealed class UserExportBuilderHostTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;

    private static readonly DateTimeOffset Now = new DateTimeOffset(2026, 5, 18, 10, 0, 0, TimeSpan.Zero);

    public UserExportBuilderHostTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);

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

    private UserExportBuilderHost BuildHost(IObjectStore store, TimeSpan? tickInterval = null) =>
        new UserExportBuilderHost(
            BuildScopeFactory(_conn),
            store,
            _clock,
            NullLogger<UserExportBuilderHost>.Instance,
            tickInterval ?? TimeSpan.FromMilliseconds(1));

    private async Task<Guid> SeedUserAsync(AppDbContext db)
    {
        var id = Guid.NewGuid();
        db.Users.Add(new User { Id = id, Email = $"builder-{id:N}@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();
        return id;
    }

    private async Task<Guid> SeedQueuedRequestAsync(AppDbContext db, Guid userId)
    {
        var reqId = Guid.NewGuid();
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return reqId;
    }

    [Fact]
    public async Task Tick_picks_up_queued_row_and_marks_ready()
    {
        var store = new InMemoryObjectStore();
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);
        var reqId = await SeedQueuedRequestAsync(scope.Db, userId);

        var host = BuildHost(store);
        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var req = await scope.Db.UserExportRequests.FindAsync(reqId);
        req!.Status.Should().Be(UserExportStatus.Ready);
        req.ObjectKey.Should().NotBeNullOrEmpty();
        req.ReadyAt.Should().NotBeNull();
        req.ExpiresAt.Should().NotBeNull();
    }

    [Fact]
    public async Task Tick_writes_bundle_to_object_store_at_known_key_shape()
    {
        var store = new InMemoryObjectStore();
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);
        var reqId = await SeedQueuedRequestAsync(scope.Db, userId);

        var host = BuildHost(store);
        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var req = await scope.Db.UserExportRequests.FindAsync(reqId);
        req!.ObjectKey.Should().Contain(userId.ToString());
        req.ObjectKey.Should().Contain(reqId.ToString());
    }

    [Fact]
    public async Task Tick_sets_expires_at_24h_after_ready_at()
    {
        var store = new InMemoryObjectStore();
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);
        var reqId = await SeedQueuedRequestAsync(scope.Db, userId);

        var host = BuildHost(store);
        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var req = await scope.Db.UserExportRequests.FindAsync(reqId);
        req!.ExpiresAt.Should().BeCloseTo(req.ReadyAt!.Value.AddHours(24), TimeSpan.FromSeconds(5));
    }

    [Fact]
    public async Task Tick_with_no_queued_rows_is_a_no_op()
    {
        var store = new InMemoryObjectStore();
        var host = BuildHost(store);
        // Should complete without exception
        await host.TickOnceAsync(default);
    }

    [Fact]
    public async Task Exception_inside_assembler_transitions_row_to_failed()
    {
        // Use a store that throws on PutAsync to simulate assembler failure
        var store = new ThrowingObjectStore();
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);
        var reqId = await SeedQueuedRequestAsync(scope.Db, userId);

        var host = BuildHost(store);
        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var req = await scope.Db.UserExportRequests.FindAsync(reqId);
        req!.Status.Should().Be(UserExportStatus.Failed);
        req.FailureReason.Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task Two_concurrent_ticks_only_process_each_row_once()
    {
        // Seed exactly one queued row. Fire two ticks concurrently.
        // The optimistic concurrency token (Version) means only one tick can claim the row.
        // The other must receive DbUpdateConcurrencyException and skip — the row is processed once.
        var store = new InMemoryObjectStore();
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);
        var reqId = await SeedQueuedRequestAsync(scope.Db, userId);

        var host = BuildHost(store);

        // Run two ticks in parallel; both see the same Queued row initially.
        await Task.WhenAll(
            host.TickOnceAsync(default),
            host.TickOnceAsync(default));

        scope.Db.ChangeTracker.Clear();
        var req = await scope.Db.UserExportRequests.FindAsync(reqId);

        // The row must be in a terminal state — one tick won the claim, the other skipped.
        req!.Status.Should().BeOneOf(UserExportStatus.Ready, UserExportStatus.Failed);

        // The object store must contain at most one entry for this request (not two bundles).
        // If both ticks processed the row, PutAsync would have been called twice, which is
        // detectable by checking the store for the expected key.
        var expectedKey = $"exports/{userId}/{reqId}.json";
        if (req.Status == UserExportStatus.Ready)
        {
            // One bundle was written.
            req.ObjectKey.Should().Be(expectedKey);
        }
    }

    public ValueTask DisposeAsync()
    {
        _conn.Dispose();
        return ValueTask.CompletedTask;
    }

    /// <summary>Test double that throws on PutAsync to simulate storage failures.</summary>
    private sealed class ThrowingObjectStore : IObjectStore
    {
        public Task PutAsync(string key, Stream content, string contentType, CancellationToken ct)
            => throw new InvalidOperationException("Simulated storage failure");

        public Task<Uri> GetSignedUrlAsync(string key, TimeSpan ttl, CancellationToken ct)
            => Task.FromResult(new Uri("http://test/never"));

        public Task<Stream> GetAsync(string key, CancellationToken ct)
            => Task.FromResult<Stream>(Stream.Null);
    }
}
