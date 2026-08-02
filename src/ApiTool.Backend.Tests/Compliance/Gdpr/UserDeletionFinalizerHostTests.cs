using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Unit tests for <see cref="UserDeletionFinalizerHost.TickOnceAsync"/>.
/// Uses SQLite in-memory for realistic EF Core query behaviour.
/// </summary>
public sealed class UserDeletionFinalizerHostTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;
    private readonly FakeClock _clock;
    private readonly RecordingEmailQueue _emailQueue;

    private static readonly DateTimeOffset Now = new DateTimeOffset(2026, 5, 18, 3, 0, 0, TimeSpan.Zero);

    public UserDeletionFinalizerHostTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        _clock = new FakeClock(Now);
        _emailQueue = new RecordingEmailQueue();

        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private static IServiceScopeFactory BuildScopeFactory(SqliteConnection conn, IUserAnonymiser anonymiser)
    {
        var services = new ServiceCollection();
        services.AddDbContext<AppDbContext>(opts => opts.UseSqlite(conn));
        services.AddSingleton(anonymiser);
        return services.BuildServiceProvider().GetRequiredService<IServiceScopeFactory>();
    }

    private UserDeletionFinalizerHost BuildHost(IUserAnonymiser? anonymiser = null)
    {
        // When no anonymiser provided, use a test-local stub that sets anonymised_at on the user.
        var anon = anonymiser ?? new TestSqliteUserAnonymiser(_conn);
        return new UserDeletionFinalizerHost(
            BuildScopeFactory(_conn, anon),
            _emailQueue,
            _clock,
            NullLogger<UserDeletionFinalizerHost>.Instance,
            tickInterval: TimeSpan.FromMilliseconds(1));
    }

    private async Task<Guid> SeedUserPendingAsync(AppDbContext db, DateTime? pendingAt = null)
    {
        var id = Guid.NewGuid();
        db.Users.Add(new User
        {
            Id = id,
            Email = $"finalizer-{id:N}@example.com",
            CreatedAt = DateTime.UtcNow,
            PendingDeletionAt = pendingAt ?? Now.UtcDateTime.AddDays(-31),
        });
        await db.SaveChangesAsync();
        return id;
    }

    [Fact]
    public async Task Tick_with_no_pending_users_is_a_noop()
    {
        var host = BuildHost();

        // Seed a user without pending deletion
        using var scope = OpenScope();
        scope.Db.Users.Add(new User
        {
            Id = Guid.NewGuid(),
            Email = "nopending@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        await host.TickOnceAsync(default);

        _emailQueue.Messages.Should().BeEmpty(because: "no users past cooldown");
    }

    [Fact]
    public async Task Tick_skips_users_whose_pending_deletion_at_is_less_than_30_days_old()
    {
        var host = BuildHost();
        using var scope = OpenScope();
        var userId = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-5));

        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var user = await scope.Db.Users.FindAsync(userId);
        user!.AnonymisedAt.Should().BeNull(because: "cooldown not elapsed (only 5 days)");
        _emailQueue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task Tick_invokes_anonymiser_for_users_past_30_day_cooldown()
    {
        var anonymiser = new SpyUserAnonymiser();
        var host = BuildHost(anonymiser);
        using var scope = OpenScope();
        var userId = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-31));

        await host.TickOnceAsync(default);

        anonymiser.AnonymisedUserIds.Should().Contain(userId,
            because: "user is 31 days past pending_deletion_at");
    }

    [Fact]
    public async Task Tick_sets_anonymised_at_for_processed_users()
    {
        var host = BuildHost();
        using var scope = OpenScope();
        var userId = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-31));

        await host.TickOnceAsync(default);

        scope.Db.ChangeTracker.Clear();
        var user = await scope.Db.Users.FindAsync(userId);
        user!.AnonymisedAt.Should().NotBeNull(because: "stub anonymiser sets anonymised_at");
        user.PendingDeletionAt.Should().BeNull(because: "stub clears pending_deletion_at after anonymisation");
    }

    [Fact]
    public async Task Tick_enqueues_account_deletion_completed_email()
    {
        _emailQueue.Clear();
        var host = BuildHost();
        using var scope = OpenScope();
        var userId = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-31));

        await host.TickOnceAsync(default);

        var emails = _emailQueue.Messages
            .Where(m => m.TemplateSlug == "account_deletion_completed")
            .ToList();
        emails.Should().ContainSingle(because: "finalizer must enqueue account_deletion_completed email");
    }

    [Fact]
    public async Task Tick_continues_after_one_user_throws()
    {
        var throwingAnonymiser = new ThrowingUserAnonymiser(failOnFirst: true);
        var host = BuildHost(throwingAnonymiser);

        using var scope = OpenScope();
        var userA = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-31));
        var userB = await SeedUserPendingAsync(scope.Db, pendingAt: Now.UtcDateTime.AddDays(-31));

        // Should not throw — exceptions per user are caught and logged
        await host.TickOnceAsync(default);

        // throwingAnonymiser only throws on first user; second user processed
        throwingAnonymiser.CallCount.Should().Be(2,
            because: "finalizer must process all users even if one throws");
    }

    [Fact]
    public void InternalRunDeletionFinalizer_host_exposes_TickOnceAsync_method()
    {
        // Verifies that UserDeletionFinalizerHost has a public TickOnceAsync method so the
        // InternalRunDeletionFinalizerEndpoint can drive it synchronously in tests.
        var method = typeof(UserDeletionFinalizerHost)
            .GetMethod(nameof(UserDeletionFinalizerHost.TickOnceAsync));
        method.Should().NotBeNull(
            because: "UserDeletionFinalizerHost.TickOnceAsync must be public for the test-hook endpoint");
    }
}

/// <summary>
/// Test-local stub that sets anonymised_at directly via the shared SQLite connection.
/// Mirrors what StubUserAnonymiser does in production DI but uses the shared connection.
/// </summary>
file sealed class TestSqliteUserAnonymiser(SqliteConnection conn) : IUserAnonymiser
{
    public async Task AnonymiseAsync(Guid userId, CancellationToken ct)
    {
        var opts = new DbContextOptionsBuilder<AppDbContext>().UseSqlite(conn).Options;
        await using var db = new AppDbContext(opts);
        var user = await db.Users.FindAsync([userId], ct);
        if (user is null) return;
        user.AnonymisedAt = DateTime.UtcNow;
        user.PendingDeletionAt = null;
        await db.SaveChangesAsync(ct);
    }
}

/// <summary>Spy anonymiser that records which user IDs it was called with.</summary>
file sealed class SpyUserAnonymiser : IUserAnonymiser
{
    public List<Guid> AnonymisedUserIds { get; } = [];

    public Task AnonymiseAsync(Guid userId, CancellationToken ct)
    {
        AnonymisedUserIds.Add(userId);
        return Task.CompletedTask;
    }
}

/// <summary>Anonymiser that throws on the first call, succeeds on subsequent calls.</summary>
file sealed class ThrowingUserAnonymiser(bool failOnFirst) : IUserAnonymiser
{
    public int CallCount { get; private set; }
    private bool _hasFailed;

    public Task AnonymiseAsync(Guid userId, CancellationToken ct)
    {
        CallCount++;
        if (failOnFirst && !_hasFailed)
        {
            _hasFailed = true;
            throw new InvalidOperationException("Simulated anonymiser failure");
        }

        return Task.CompletedTask;
    }
}
