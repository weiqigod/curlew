using System.Text.Json;
using Microsoft.Data.Sqlite;

namespace ApiTool.Backend.Bootstrap;

/// <summary>
/// Catches database exceptions that indicate a missing schema (table does not exist) and
/// translates them into HTTP 503 Service Unavailable responses. This guards the case where
/// <c>BACKEND_RUN_MIGRATIONS=0</c> and the database schema has not yet been applied.
/// </summary>
/// <remarks>
/// Recognized error patterns:
/// <list type="bullet">
///   <item>SQLite error 1 — "no such table" (SqliteException.SqliteErrorCode == 1 and message contains "no such table")</item>
///   <item>Npgsql SqlState 42P01 — "undefined_table"</item>
/// </list>
/// </remarks>
public sealed class SchemaGuardMiddleware(RequestDelegate next)
{
    private static readonly JsonSerializerOptions SerializerOptions =
        new() { PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower };

    /// <inheritdoc cref="IMiddleware.InvokeAsync"/>
    public async Task InvokeAsync(HttpContext context)
    {
        try
        {
            await next(context);
        }
        catch (Exception ex) when (IsMissingSchemaException(ex))
        {
            context.Response.StatusCode = StatusCodes.Status503ServiceUnavailable;
            context.Response.ContentType = "application/json";
            var body = JsonSerializer.Serialize(
                new { code = "service_unavailable", message = "Database schema not applied. Set BACKEND_RUN_MIGRATIONS=1 and restart." },
                SerializerOptions);
            await context.Response.WriteAsync(body);
        }
    }

    /// <summary>
    /// Determines whether an exception indicates a missing database table (schema not applied).
    /// Checks the exception chain so EF-wrapped exceptions are also caught.
    /// </summary>
    public static bool IsMissingSchemaException(Exception ex)
    {
        var current = ex;
        while (current is not null)
        {
            if (IsSqliteMissingTable(current) || IsNpgsqlUndefinedTable(current))
                return true;
            current = current.InnerException;
        }
        return false;
    }

    private static bool IsSqliteMissingTable(Exception ex)
    {
        // SqliteException.SqliteErrorCode == 1 (SQLITE_ERROR) with "no such table" in the message
        return ex is SqliteException sqlite
            && sqlite.SqliteErrorCode == 1
            && sqlite.Message.Contains("no such table", StringComparison.OrdinalIgnoreCase);
    }

    private static bool IsNpgsqlUndefinedTable(Exception ex)
    {
        // Npgsql throws PostgresException with SqlState "42P01" for undefined_table.
        // We check by type name to avoid a hard Npgsql assembly dependency in this file
        // (Npgsql is already a transitive dependency via EF Core provider).
        var typeName = ex.GetType().FullName ?? string.Empty;
        if (!typeName.Equals("Npgsql.PostgresException", StringComparison.Ordinal))
            return false;
        var sqlStateProperty = ex.GetType().GetProperty("SqlState");
        var sqlState = sqlStateProperty?.GetValue(ex) as string;
        return sqlState == "42P01";
    }
}
