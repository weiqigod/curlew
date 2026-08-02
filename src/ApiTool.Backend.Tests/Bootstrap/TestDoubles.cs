using ApiTool.Backend.Data;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.EntityFrameworkCore.Diagnostics;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Bootstrap;

/// <summary>Captures log entries for assertions in bootstrap unit tests.</summary>
internal sealed class TestLogger<T> : ILogger<T>
{
    private readonly List<LogEntry> _entries = [];

    /// <summary>All entries logged so far.</summary>
    public IReadOnlyList<LogEntry> Entries => _entries;

    /// <inheritdoc/>
    public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;

    /// <inheritdoc/>
    public bool IsEnabled(LogLevel logLevel) => true;

    /// <inheritdoc/>
    public void Log<TState>(
        LogLevel logLevel,
        EventId eventId,
        TState state,
        Exception? exception,
        Func<TState, Exception?, string> formatter)
    {
        _entries.Add(new LogEntry(logLevel, formatter(state, exception)));
    }
}

/// <summary>A captured log entry.</summary>
internal sealed record LogEntry(LogLevel Level, string Message);

/// <summary>An <see cref="IServiceScopeFactory"/> that resolves a single pre-built <see cref="AppDbContext"/>.</summary>
internal sealed class SingletonScopeFactory(AppDbContext db) : IServiceScopeFactory
{
    /// <inheritdoc/>
    public IServiceScope CreateScope() => new Scope(db);

    private sealed class Scope(AppDbContext db) : IServiceScope
    {
        public IServiceProvider ServiceProvider { get; } = new SingletonServiceProvider(db);
        public void Dispose() { }
    }

    private sealed class SingletonServiceProvider(AppDbContext db) : IServiceProvider
    {
        public object? GetService(Type serviceType)
        {
            if (serviceType == typeof(AppDbContext)) return db;
            if (serviceType == typeof(TrialSeederService))
                return new TrialSeederService(db, TimeProvider.System,
                    NullLogger<TrialSeederService>.Instance);
            return null;
        }
    }
}

/// <summary>Provides a stub <see cref="IServiceScopeFactory"/> with no services — for skip-path tests.</summary>
internal static class StubScopes
{
    /// <summary>A no-op scope factory (MigrationRunner never resolves the scope on the skip path).</summary>
    public static IServiceScopeFactory Empty { get; } = new EmptyScopeFactory();

    private sealed class EmptyScopeFactory : IServiceScopeFactory
    {
        public IServiceScope CreateScope() => new EmptyScope();
    }

    private sealed class EmptyScope : IServiceScope
    {
        public IServiceProvider ServiceProvider => EmptyServiceProvider.Instance;
        public void Dispose() { }
    }

    private sealed class EmptyServiceProvider : IServiceProvider
    {
        public static readonly EmptyServiceProvider Instance = new();
        public object? GetService(Type serviceType) => null;
    }
}

/// <summary>
/// Creates an <see cref="AppDbContext"/> backed by an in-memory SQLite DB that has
/// already been deleted — EF migrations will therefore create it fresh.
/// </summary>
internal static class FreshSqliteDb
{
    public static (AppDbContext Db, IAsyncDisposable Scope) Create()
    {
        var scope = TestDb.CreateOpen();
        return (scope.Db, scope);
    }
}

/// <summary>
/// An EF Core command interceptor that delays every command execution by the specified duration.
/// Used to simulate a slow database for timeout testing.
/// </summary>
internal sealed class DelayInterceptor(TimeSpan delay) : DbCommandInterceptor
{
    public override async ValueTask<InterceptionResult<System.Data.Common.DbDataReader>> ReaderExecutingAsync(
        System.Data.Common.DbCommand command,
        CommandEventData eventData,
        InterceptionResult<System.Data.Common.DbDataReader> result,
        CancellationToken cancellationToken = default)
    {
        await Task.Delay(delay, cancellationToken);
        return result;
    }

    public override async ValueTask<InterceptionResult<int>> NonQueryExecutingAsync(
        System.Data.Common.DbCommand command,
        CommandEventData eventData,
        InterceptionResult<int> result,
        CancellationToken cancellationToken = default)
    {
        await Task.Delay(delay, cancellationToken);
        return result;
    }
}

/// <summary>
/// An EF Core save-changes interceptor that throws <see cref="InvalidOperationException"/> on every
/// <c>SavingChanges</c> call. Used to force the transaction rollback path in
/// <see cref="ApiTool.Backend.Bootstrap.AdminBootstrap.RunAsync"/>.
/// </summary>
internal sealed class SaveFailureInterceptor : SaveChangesInterceptor
{
    /// <inheritdoc/>
    public override ValueTask<InterceptionResult<int>> SavingChangesAsync(
        DbContextEventData eventData,
        InterceptionResult<int> result,
        CancellationToken cancellationToken = default)
        => throw new InvalidOperationException("Simulated SaveChanges failure for rollback test.");
}

/// <summary>
/// Creates an <see cref="IServiceScopeFactory"/> backed by a SQLite DB whose <c>SaveChangesAsync</c>
/// always throws — for testing the <see cref="ApiTool.Backend.Bootstrap.AdminBootstrap"/> rollback path.
/// </summary>
internal static class FailOnSaveScopeFactory
{
    /// <summary>
    /// Creates a scope factory that uses the given open SQLite connection with a
    /// <see cref="SaveFailureInterceptor"/> wired in. The schema must be created before calling bootstrap.
    /// </summary>
    public static SingletonScopeFactory Create(SqliteConnection conn)
    {
        var opts = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(conn)
            .AddInterceptors(new SaveFailureInterceptor())
            .Options;
        var ctx = new AppDbContext(opts);
        return new SingletonScopeFactory(ctx);
    }
}

/// <summary>
/// Creates an <see cref="IServiceScopeFactory"/> that returns an <see cref="AppDbContext"/>
/// backed by a SQLite DB with an artificial command delay — for testing timeout behaviour.
/// </summary>
internal static class SlowScopeFactory
{
    /// <summary>
    /// Creates a scope factory for an empty SQLite DB whose commands are delayed by <paramref name="commandDelay"/>.
    /// The returned <see cref="IAsyncDisposable"/> must be disposed after the test.
    /// </summary>
    public static (IServiceScopeFactory Factory, IAsyncDisposable Cleanup) Create(TimeSpan commandDelay)
    {
        var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        var opts = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(conn)
            .AddInterceptors(new DelayInterceptor(commandDelay))
            .Options;
        var ctx = new AppDbContext(opts);
        var factory = new SingletonScopeFactory(ctx);
        return (factory, new CompositeDisposable(ctx, conn));
    }

    private sealed class CompositeDisposable(AppDbContext ctx, SqliteConnection conn) : IAsyncDisposable
    {
        public async ValueTask DisposeAsync()
        {
            await ctx.DisposeAsync();
            await conn.DisposeAsync();
        }
    }
}
