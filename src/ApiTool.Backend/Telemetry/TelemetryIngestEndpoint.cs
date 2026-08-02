using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// Registers <c>POST /api/v1/telemetry/events</c> — an anonymous, idempotent telemetry
/// ingest endpoint for CLI-emitted events. No authentication required per the v4-9
/// opt-in consent model.
/// <para>
/// Constraints enforced by this endpoint:
/// <list type="bullet">
///   <item>Body ≤ 64 KB (413 if exceeded).</item>
///   <item><c>Idempotency-Key</c> header required; must be ≤ 64 chars (400 if missing).</item>
///   <item><c>install_id</c> must be a valid UUID (400 if not).</item>
///   <item><c>event_type</c> must be a non-empty string ≤ 64 chars (400 if not).</item>
///   <item>Duplicate <c>idempotency_key</c> → 202 no-op (at-most-once delivery).</item>
///   <item>Per-<c>install_id</c> rate limit: 60/min → 429 if exceeded.</item>
/// </list>
/// </para>
/// Refs: docs/SPECIFICATION.md — Telemetry Phase 3 Implementation Pipeline.
/// </summary>
public static class TelemetryIngestEndpoint
{
    /// <summary>Maximum body size accepted by this endpoint (64 KB).</summary>
    public const int MaxBodyBytes = 64 * 1024;

    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    /// <summary>Maps the telemetry ingest endpoint onto the route builder.</summary>
    public static IEndpointRouteBuilder MapTelemetryIngestEndpoint(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/telemetry/events", HandleAsync)
            .AllowAnonymous()
            .DisableAntiforgery()
            .RequireRateLimiting("telemetry-ingest")
            .WithName("TelemetryIngest")
            .WithTags("Telemetry")
            .Accepts<TelemetryIngestRequest>("application/json")
            .Produces(StatusCodes.Status202Accepted)
            .ProducesProblem(StatusCodes.Status400BadRequest)
            .ProducesProblem(StatusCodes.Status413RequestEntityTooLarge)
            .ProducesProblem(StatusCodes.Status429TooManyRequests);

        return app;
    }

    private static async Task<IResult> HandleAsync(
        HttpContext context,
        AppDbContext db,
        TimeProvider clock,
        CancellationToken ct)
    {
        var request = context.Request;

        // ── 1. 64KB body cap ─────────────────────────────────────────────────────
        if (request.ContentLength.HasValue && request.ContentLength.Value > MaxBodyBytes)
            return HttpResults.Problem(
                title: "Payload Too Large",
                detail: "Request body must not exceed 64 KB.",
                statusCode: StatusCodes.Status413RequestEntityTooLarge);

        // ── 2. Idempotency-Key header ────────────────────────────────────────────
        if (!request.Headers.TryGetValue("Idempotency-Key", out var keyValues) ||
            string.IsNullOrWhiteSpace(keyValues.ToString()))
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "The 'Idempotency-Key' header is required.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        var idempotencyKey = keyValues.ToString().Trim();
        if (idempotencyKey.Length > 64)
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "The 'Idempotency-Key' header must not exceed 64 characters.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        // ── 3. Read + size-cap body ───────────────────────────────────────────────
        request.EnableBuffering();
        using var bodyReader = new System.IO.StreamReader(request.Body, leaveOpen: true);
        var rawBody = await bodyReader.ReadToEndAsync(ct);

        if (rawBody.Length > MaxBodyBytes)
        {
            return HttpResults.Problem(
                title: "Payload Too Large",
                detail: "Request body must not exceed 64 KB.",
                statusCode: StatusCodes.Status413RequestEntityTooLarge);
        }

        // ── 4. Parse body ─────────────────────────────────────────────────────────
        TelemetryIngestRequest? body;
        try
        {
            body = JsonSerializer.Deserialize<TelemetryIngestRequest>(rawBody, SnakeCaseOptions);
        }
        catch (JsonException)
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "Request body is not valid JSON.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        if (body is null)
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "Request body is required.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        // ── 5. Validate install_id ────────────────────────────────────────────────
        if (string.IsNullOrWhiteSpace(body.InstallId) ||
            !Guid.TryParse(body.InstallId, out var installId))
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "The 'install_id' field must be a valid UUID.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        // ── 6. Validate event_type ────────────────────────────────────────────────
        if (string.IsNullOrWhiteSpace(body.EventType) || body.EventType.Length > 64)
        {
            return HttpResults.Problem(
                title: "Bad Request",
                detail: "The 'event_type' field must be a non-empty string of at most 64 characters.",
                statusCode: StatusCodes.Status400BadRequest);
        }

        // ── 7. Normalise payload ──────────────────────────────────────────────────
        var payloadJson = body.EventPayload?.ToJsonString() ?? "{}";

        // ── 8. Insert with idempotency guard ──────────────────────────────────────
        // Pre-check: if the idempotency key is already present, return 202 no-op.
        // This covers both the EF InMemory provider (no UNIQUE constraint enforcement)
        // and acts as a fast-path before hitting the DB-level constraint.
        var duplicate = await db.TelemetryEvents
            .AnyAsync(e => e.IdempotencyKey == idempotencyKey, ct);
        if (duplicate)
            return HttpResults.StatusCode(StatusCodes.Status202Accepted);

        var ev = new TelemetryEvent
        {
            Id = Guid.NewGuid(),
            InstallId = installId,
            EventType = body.EventType,
            EventPayloadJson = payloadJson,
            ReceivedAt = clock.GetUtcNow().UtcDateTime,
            IdempotencyKey = idempotencyKey,
        };

        db.TelemetryEvents.Add(ev);

        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateException ex) when (IsUniqueConstraintViolation(ex))
        {
            // Race-condition safety: if two concurrent requests slipped past the pre-check,
            // the DB-level UNIQUE constraint catches the second one.
            return HttpResults.StatusCode(StatusCodes.Status202Accepted);
        }

        return HttpResults.StatusCode(StatusCodes.Status202Accepted);
    }

    /// <summary>
    /// Returns <see langword="true"/> if <paramref name="ex"/> represents a UNIQUE constraint
    /// violation on any provider (SQLite SQLITE_CONSTRAINT_UNIQUE or Postgres 23505).
    /// </summary>
    private static bool IsUniqueConstraintViolation(DbUpdateException ex)
    {
        // SQLite: Microsoft.Data.Sqlite.SqliteException with SqliteErrorCode == 19 (SQLITE_CONSTRAINT).
        if (ex.InnerException is Microsoft.Data.Sqlite.SqliteException sqe)
            return sqe.SqliteErrorCode == 19;

        // Postgres: Npgsql.PostgresException with SqlState == "23505" (unique_violation).
        // Use reflection to avoid a compile-time reference to Npgsql — EF Core ships it
        // as a transitive dependency; the type name is stable across Npgsql versions.
        // This avoids the RuntimeBinderException risk of `dynamic` while keeping the
        // project free of an explicit Npgsql package reference.
        var innerType = ex.InnerException?.GetType().FullName ?? string.Empty;
        if (innerType.Contains("PostgresException"))
        {
            var sqlState = ex.InnerException!
                .GetType()
                .GetProperty("SqlState")
                ?.GetValue(ex.InnerException) as string;
            return sqlState == "23505";
        }

        return false;
    }
}
