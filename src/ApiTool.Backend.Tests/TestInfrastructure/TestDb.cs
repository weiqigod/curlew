using ApiTool.Backend.Data;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>Creates an in-memory SQLite database backed by a kept-open connection for per-test isolation.</summary>
internal static class TestDb
{
    /// <summary>
    /// Creates and opens a new in-memory <see cref="AppDbContext"/> wrapped in a
    /// <see cref="TestDbScope"/> that disposes both the context and its underlying
    /// <see cref="SqliteConnection"/> when the scope is disposed.
    /// </summary>
    public static TestDbScope CreateOpen()
    {
        var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        var opts = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(conn)
            .Options;
        var ctx = new AppDbContext(opts);
        return new TestDbScope(conn, ctx);
    }

    /// <summary>
    /// Creates an <see cref="AppDbContext"/> backed by an already-open <see cref="SqliteConnection"/>.
    /// The returned <see cref="TestDbScope"/> does NOT own the connection — the caller is responsible
    /// for disposing the connection. Disposing the scope only disposes the <see cref="AppDbContext"/>.
    /// This overload allows multiple <see cref="AppDbContext"/> instances to share one SQLite DB.
    /// </summary>
    public static TestDbScope CreateOpenFromConnection(SqliteConnection conn)
    {
        var opts = new DbContextOptionsBuilder<AppDbContext>()
            .UseSqlite(conn)
            .Options;
        var ctx = new AppDbContext(opts);
        // Pass a dummy connection that owns nothing so the scope doesn't close the shared conn.
        return new TestDbScope(conn, ctx, ownsConnection: false);
    }
}

/// <summary>
/// Holds a <see cref="SqliteConnection"/> and its associated <see cref="AppDbContext"/>.
/// Disposing this scope closes and disposes both resources (unless <c>ownsConnection</c> is false).
/// </summary>
internal sealed class TestDbScope : IAsyncDisposable, IDisposable
{
    private readonly SqliteConnection _connection;
    private readonly bool _ownsConnection;

    internal TestDbScope(SqliteConnection connection, AppDbContext context, bool ownsConnection = true)
    {
        _connection = connection;
        _ownsConnection = ownsConnection;
        Db = context;
    }

    /// <summary>Gets the <see cref="AppDbContext"/> backed by the in-memory SQLite connection.</summary>
    public AppDbContext Db { get; }

    /// <summary>
    /// Gets the underlying <see cref="SqliteConnection"/> so tests can share it across
    /// multiple contexts (e.g. interceptors that insert via raw ADO.NET on the same
    /// in-memory database).
    /// </summary>
    internal SqliteConnection Connection => _connection;

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        await Db.DisposeAsync();
        if (_ownsConnection)
            await _connection.DisposeAsync();
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        Db.Dispose();
        if (_ownsConnection)
            _connection.Dispose();
    }
}
