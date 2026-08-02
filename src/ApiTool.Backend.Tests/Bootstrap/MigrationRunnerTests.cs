using ApiTool.Backend.Bootstrap;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Tests.Bootstrap;

/// <summary>Unit tests for the migration runner startup task.</summary>
public sealed class MigrationRunnerTests
{
    [Fact]
    public async Task RunAsync_run_false_logs_skipped_and_returns_zero()
    {
        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(StubScopes.Empty, sink);
        var count = await runner.RunAsync(run: false);
        count.Should().Be(0);
        sink.Entries.Should().ContainSingle(e => e.Message.Contains("migrations: skipped"));
    }

    [Fact]
    public async Task RunAsync_with_fresh_sqlite_applies_all_pending_migrations()
    {
        await using var scope = TestDb.CreateOpen();
        // Ensure a clean DB without schema so all migrations are pending.
        await scope.Db.Database.EnsureDeletedAsync();

        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(
            new SingletonScopeFactory(scope.Db), sink);

        var count = await runner.RunAsync(run: true);
        count.Should().BeGreaterThan(0);
        sink.Entries.Should().Contain(e => e.Message.StartsWith("migrations: applied "));
    }

    [Fact]
    public async Task RunAsync_already_up_to_date_returns_zero_with_log()
    {
        await using var scope = TestDb.CreateOpen();
        // Apply all migrations first.
        await scope.Db.Database.MigrateAsync();

        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(
            new SingletonScopeFactory(scope.Db), sink);

        var count = await runner.RunAsync(run: true);
        count.Should().Be(0);
        sink.Entries.Should().Contain(e => e.Message.Contains("up-to-date"));
    }

    [Fact]
    public async Task RunAsync_propagates_DB_failure_as_MigrationFailedException()
    {
        // Use a context configured with an invalid connection string to simulate failure.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureDeletedAsync();
        await scope.DisposeAsync(); // close the connection — further operations will fail

        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(
            new SingletonScopeFactory(scope.Db), sink);

        var act = async () => await runner.RunAsync(run: true);
        await act.Should().ThrowAsync<MigrationFailedException>();
        sink.Entries.Should().ContainSingle(e =>
            e.Level == LogLevel.Error && e.Message.Contains("migrations: failed"));
    }

    [Fact]
    public async Task RunAsync_timeout_yields_MigrationFailedException()
    {
        // Create a scope factory whose DB commands are delayed by 5 s, then pass a 50 ms timeout.
        // The command-level cancellation fires before the operation completes, which causes
        // OperationCanceledException to be caught and wrapped as MigrationFailedException.
        var (factory, cleanup) = SlowScopeFactory.Create(TimeSpan.FromSeconds(5));
        await using var _ = cleanup;

        var sink = new TestLogger<MigrationRunner>();
        var runner = new MigrationRunner(factory, sink);

        var act = async () => await runner.RunAsync(run: true, timeout: TimeSpan.FromMilliseconds(50));
        await act.Should().ThrowAsync<MigrationFailedException>();
        sink.Entries.Should().ContainSingle(e =>
            e.Level == LogLevel.Error && e.Message.Contains("migrations: failed"));
    }
}
