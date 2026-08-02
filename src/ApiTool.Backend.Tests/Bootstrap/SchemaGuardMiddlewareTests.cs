using System.Net;
using System.Net.Http.Json;
using ApiTool.Backend.Bootstrap;
using ApiTool.Backend.Data;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.Bootstrap;

/// <summary>
/// Integration test that exercises the full <see cref="SchemaGuardMiddleware"/> HTTP pipeline:
/// when a request hits an endpoint backed by a schema-less database, the middleware must
/// intercept the missing-table exception and return HTTP 503 with a structured JSON body.
/// </summary>
public sealed class SchemaGuardMiddlewareHttpTests : IAsyncLifetime
{
    // A dedicated in-memory SQLite connection whose schema is intentionally NOT created.
    private SqliteConnection? _schemalessConn;
    private HttpClient? _client;
    private WebApplicationFactory<Program>? _factory;

    /// <inheritdoc/>
    public Task InitializeAsync()
    {
        _schemalessConn = new SqliteConnection("DataSource=:memory:");
        _schemalessConn.Open();

        var conn = _schemalessConn; // capture for lambda

        _factory = new WebApplicationFactory<Program>()
            .WithWebHostBuilder(hostBuilder =>
            {
                hostBuilder.UseEnvironment("Testing");

                hostBuilder.ConfigureAppConfiguration((_, cfg) =>
                    cfg.AddInMemoryCollection(new Dictionary<string, string?>
                    {
                        ["Jwt:SigningKey"] = "test-signing-key-for-integration-tests-32bytes!",
                        ["Jwt:Issuer"]     = "apitool-test",
                        ["Jwt:Audience"]   = "apitool-test",
                        // Required to satisfy AppOptions ValidateOnStart.
                        ["ApiTool:App:WebAppUrl"] = "http://web.test",
                    }));

                hostBuilder.ConfigureServices(services =>
                {
                    // Replace the DB with an empty SQLite DB — schema deliberately not created.
                    services.RemoveAll<DbContextOptions<AppDbContext>>();
                    services.RemoveAll<AppDbContext>();
                    services.AddDbContext<AppDbContext>(options =>
                        options.UseSqlite(conn));
                });
            });

        _client = _factory.CreateClient();
        return Task.CompletedTask;
    }

    /// <inheritdoc/>
    public async Task DisposeAsync()
    {
        if (_factory is not null) await _factory.DisposeAsync();
        if (_schemalessConn is not null) await _schemalessConn.DisposeAsync();
    }

    [Fact]
    public async Task Request_to_db_backed_endpoint_with_missing_schema_returns_503()
    {
        // POST /api/v1/auth/login is AllowAnonymous and queries db.Users — it will hit
        // "no such table: users" when the schema has not been applied.
        var resp = await _client!.PostAsJsonAsync(
            "/api/v1/auth/login",
            new { email = "test@example.com", password = "ChangeMe!Password" });

        resp.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("service_unavailable");
        body.Should().Contain("BACKEND_RUN_MIGRATIONS=1");
    }
}

/// <summary>Unit tests for the SchemaGuardMiddleware exception-detection logic.</summary>
public sealed class SchemaGuardMiddlewareTests
{
    [Fact]
    public void IsMissingSchemaException_returns_true_for_sqlite_no_such_table()
    {
        // SQLite error code 1 = SQLITE_ERROR; EF Core wraps it with "no such table" in the message.
        // We simulate by creating the exception via reflection since SqliteException ctor is internal.
        var inner = CreateSqliteNoSuchTableException();
        SchemaGuardMiddleware.IsMissingSchemaException(inner).Should().BeTrue();
    }

    [Fact]
    public void IsMissingSchemaException_returns_true_for_wrapped_sqlite_exception()
    {
        var sqlite = CreateSqliteNoSuchTableException();
        var wrapped = new InvalidOperationException("EF wrapper", sqlite);
        SchemaGuardMiddleware.IsMissingSchemaException(wrapped).Should().BeTrue();
    }

    [Fact]
    public void IsMissingSchemaException_returns_false_for_generic_exception()
    {
        SchemaGuardMiddleware.IsMissingSchemaException(new InvalidOperationException("some error"))
            .Should().BeFalse();
    }

    [Fact]
    public void IsMissingSchemaException_returns_false_for_generic_db_exception()
    {
        // A non-schema exception should not be caught
        var ex = new InvalidOperationException("Connection timeout or other DB error");
        SchemaGuardMiddleware.IsMissingSchemaException(ex).Should().BeFalse();
    }

    [Fact]
    public void IsMissingSchemaException_returns_false_for_sqlite_constraint_violation()
    {
        // SQLite UNIQUE constraint error — not a missing-schema error
        var ex = CreateSqliteUniqueConstraintException();
        SchemaGuardMiddleware.IsMissingSchemaException(ex).Should().BeFalse();
    }

    // Helper: create a real SqliteException via a failed SQLite operation.
    private static SqliteException CreateSqliteNoSuchTableException()
    {
        using var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        using var cmd = conn.CreateCommand();
        cmd.CommandText = "SELECT * FROM nonexistent_table_xyz";
        try
        {
            cmd.ExecuteReader();
        }
        catch (SqliteException ex)
        {
            return ex;
        }
        throw new InvalidOperationException("Expected SqliteException was not thrown.");
    }

    private static SqliteException CreateSqliteUniqueConstraintException()
    {
        using var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        using var setup = conn.CreateCommand();
        setup.CommandText = "CREATE TABLE t (id INTEGER PRIMARY KEY)";
        setup.ExecuteNonQuery();
        using var ins1 = conn.CreateCommand();
        ins1.CommandText = "INSERT INTO t VALUES(1)";
        ins1.ExecuteNonQuery();
        using var ins2 = conn.CreateCommand();
        ins2.CommandText = "INSERT INTO t VALUES(1)";
        try
        {
            ins2.ExecuteNonQuery();
        }
        catch (SqliteException ex)
        {
            return ex;
        }
        throw new InvalidOperationException("Expected SqliteException was not thrown.");
    }
}
