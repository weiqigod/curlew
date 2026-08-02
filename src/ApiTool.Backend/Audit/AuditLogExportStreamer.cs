using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using Microsoft.AspNetCore.Http;

namespace ApiTool.Backend.Audit;

/// <summary>
/// Streams audit-log entries to <see cref="HttpResponse.Body"/> as either
/// newline-delimited JSON (JSONL) or RFC-4180 CSV. Used by the bulk-export path
/// of <see cref="AuditLogEndpoints"/>; the caller has already taken ownership of
/// the response by the time these methods are invoked.
/// </summary>
public static class AuditLogExportStreamer
{
    /// <summary>Number of rows between incremental flushes to the client.</summary>
    private const int FlushEveryRows = 100;

    private static readonly byte[] Newline = "\n"u8.ToArray();

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.Never,
    };

    /// <summary>
    /// Writes a JSONL stream — one JSON object per line — for the given org.
    /// Sets <c>Content-Type: application/x-ndjson</c>, <c>Content-Disposition: attachment</c>,
    /// and <c>X-Accel-Buffering: no</c>. Omits <c>Content-Length</c> so Kestrel
    /// uses <c>Transfer-Encoding: chunked</c>.
    /// </summary>
    public static async Task WriteJsonlAsync(
        HttpResponse response,
        string orgId,
        IAsyncEnumerable<AuditLogEntryDto> rows,
        DateTime utcNow,
        CancellationToken ct)
    {
        ConfigureResponseHeaders(response, orgId, "jsonl", "application/x-ndjson", utcNow);
        var rowCount = 0;
        await foreach (var dto in rows)
        {
            ct.ThrowIfCancellationRequested();
            var bytes = JsonSerializer.SerializeToUtf8Bytes(dto, JsonOptions);
            await response.Body.WriteAsync(bytes, ct);
            await response.Body.WriteAsync(Newline, ct);
            if (++rowCount % FlushEveryRows == 0)
                await response.Body.FlushAsync(ct);
        }
        await response.Body.FlushAsync(ct);
    }

    /// <summary>
    /// Writes a CSV stream with a header row and one data row per entry.
    /// Sets <c>Content-Type: text/csv</c>, <c>Content-Disposition: attachment</c>,
    /// and <c>X-Accel-Buffering: no</c>.
    /// </summary>
    public static async Task WriteCsvAsync(
        HttpResponse response,
        string orgId,
        IAsyncEnumerable<AuditLogEntryDto> rows,
        DateTime utcNow,
        CancellationToken ct)
    {
        ConfigureResponseHeaders(response, orgId, "csv", "text/csv", utcNow);
        await response.Body.WriteAsync(
            Encoding.UTF8.GetBytes(AuditLogCsvFormatter.HeaderRow + "\n"), ct);
        var rowCount = 0;
        await foreach (var dto in rows)
        {
            ct.ThrowIfCancellationRequested();
            var line = AuditLogCsvFormatter.FormatRow(dto) + "\n";
            await response.Body.WriteAsync(Encoding.UTF8.GetBytes(line), ct);
            if (++rowCount % FlushEveryRows == 0)
                await response.Body.FlushAsync(ct);
        }
        await response.Body.FlushAsync(ct);
    }

    private static void ConfigureResponseHeaders(
        HttpResponse response,
        string orgId,
        string ext,
        string contentType,
        DateTime utcNow)
    {
        response.StatusCode = StatusCodes.Status200OK;
        response.ContentType = contentType;
        response.Headers.ContentDisposition =
            $"attachment; filename=\"audit-log-{orgId}-{utcNow:yyyyMMddTHHmmss}.{ext}\"";
        // Hint to proxies (nginx) not to buffer — preserves chunked semantics end-to-end.
        response.Headers["X-Accel-Buffering"] = "no";
        // Do NOT set Content-Length: omitting it triggers Transfer-Encoding: chunked under Kestrel.
    }
}
