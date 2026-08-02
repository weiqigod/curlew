using System.Text;
using System.Text.Json;
using ApiTool.Backend.Audit;
using Microsoft.AspNetCore.Http;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>
/// Unit tests for <see cref="AuditLogExportStreamer"/>. Uses <see cref="DefaultHttpContext"/>
/// so Kestrel buffering is not in play — asserts header values and body content shape directly.
/// </summary>
public sealed class AuditLogExportStreamerTests
{
    private static AuditLogEntryDto MakeDto(string eventType = "test.event", string? actorId = null) =>
        new(
            EventType: eventType,
            UserId: actorId ?? Guid.NewGuid().ToString("N"),
            UserEmail: "actor@example.com",
            TargetType: "org",
            TargetId: Guid.NewGuid().ToString("N"),
            CreatedAt: new DateTime(2026, 1, 15, 12, 0, 0, DateTimeKind.Utc),
            IpAddress: "127.0.0.1",
            Success: true,
            FailureReason: null);

    private static async IAsyncEnumerable<AuditLogEntryDto> ToAsync(IEnumerable<AuditLogEntryDto> src)
    {
        foreach (var dto in src) { yield return dto; await Task.Yield(); }
    }

    private static async Task<(DefaultHttpContext ctx, byte[] body)> CaptureAsync(
        Func<DefaultHttpContext, Task> run)
    {
        var ctx = new DefaultHttpContext();
        var ms = new MemoryStream();
        ctx.Response.Body = ms;
        await run(ctx);
        return (ctx, ms.ToArray());
    }

    private static readonly DateTime FixedUtcNow = new(2026, 5, 17, 12, 34, 56, DateTimeKind.Utc);

    // ── JSONL ──────────────────────────────────────────────────────────────────

    [Fact]
    public async Task WriteJsonlAsync_sets_content_type_to_ndjson()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "deadbeef", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.ContentType.Should().Be("application/x-ndjson");
    }

    [Fact]
    public async Task WriteJsonlAsync_sets_content_disposition_attachment_with_correct_filename()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "deadbeef", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.Headers.ContentDisposition.ToString()
            .Should().Be("attachment; filename=\"audit-log-deadbeef-20260517T123456.jsonl\"");
    }

    [Fact]
    public async Task WriteJsonlAsync_sets_x_accel_buffering_no()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "org1", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.Headers["X-Accel-Buffering"].ToString().Should().Be("no");
    }

    [Fact]
    public async Task WriteJsonlAsync_does_not_set_content_length_so_chunked_encoding_applies()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "org1", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.ContentLength.Should().BeNull();
    }

    [Fact]
    public async Task WriteJsonlAsync_writes_one_json_object_per_line_with_snake_case_keys()
    {
        var dtos = Enumerable.Range(0, 3).Select(i => MakeDto($"event.{i}")).ToList();
        var (_, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "org1", ToAsync(dtos), FixedUtcNow, default));

        var text = Encoding.UTF8.GetString(body).TrimEnd('\n');
        var lines = text.Split('\n');
        lines.Should().HaveCount(3);

        foreach (var line in lines)
        {
            using var doc = JsonDocument.Parse(line);
            doc.RootElement.TryGetProperty("event_type", out _).Should().BeTrue();
            doc.RootElement.TryGetProperty("user_id", out _).Should().BeTrue();
            doc.RootElement.TryGetProperty("created_at", out _).Should().BeTrue();
        }
    }

    [Fact]
    public async Task WriteJsonlAsync_writes_350_lines_for_350_dtos()
    {
        var dtos = Enumerable.Range(0, 350).Select(i => MakeDto()).ToList();
        var (_, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(c.Response, "org1", ToAsync(dtos), FixedUtcNow, default));

        var text = Encoding.UTF8.GetString(body).TrimEnd('\n');
        text.Split('\n').Should().HaveCount(350);
    }

    [Fact]
    public async Task WriteJsonlAsync_propagates_cancellation_mid_stream()
    {
        using var cts = new CancellationTokenSource();
        var ctx = new DefaultHttpContext();
        ctx.Response.Body = new MemoryStream();

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() =>
            AuditLogExportStreamer.WriteJsonlAsync(
                ctx.Response, "org1",
                CancelAfterNAsync(5, cts),
                FixedUtcNow,
                cts.Token));
    }

    private static async IAsyncEnumerable<AuditLogEntryDto> CancelAfterNAsync(
        int cancelAfter,
        CancellationTokenSource cts,
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken ct = default)
    {
        var count = 0;
        while (true)
        {
            if (count++ >= cancelAfter) cts.Cancel();
            ct.ThrowIfCancellationRequested();
            yield return MakeDto();
            await Task.Yield();
        }
    }

    // ── CSV ────────────────────────────────────────────────────────────────────

    [Fact]
    public async Task WriteCsvAsync_sets_content_type_to_text_csv()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(c.Response, "org1", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.ContentType.Should().Be("text/csv");
    }

    [Fact]
    public async Task WriteCsvAsync_sets_content_disposition_with_csv_extension()
    {
        var (ctx, _) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(c.Response, "deadbeef", ToAsync([MakeDto()]), FixedUtcNow, default));

        ctx.Response.Headers.ContentDisposition.ToString()
            .Should().Be("attachment; filename=\"audit-log-deadbeef-20260517T123456.csv\"");
    }

    [Fact]
    public async Task WriteCsvAsync_writes_header_row_plus_one_line_per_entry()
    {
        var dtos = Enumerable.Range(0, 350).Select(_ => MakeDto()).ToList();
        var (_, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(c.Response, "org1", ToAsync(dtos), FixedUtcNow, default));

        var text = Encoding.UTF8.GetString(body).TrimEnd('\n');
        var lines = text.Split('\n');
        lines.Should().HaveCount(351); // header + 350 data rows
        lines[0].Should().Be("created_at,event_type,user_id,user_email,target_type,target_id,success,ip_address");
    }

    [Fact]
    public async Task WriteCsvAsync_quotes_field_with_comma()
    {
        var dto = MakeDto(eventType: "org,event");
        var (_, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(c.Response, "org1", ToAsync([dto]), FixedUtcNow, default));

        Encoding.UTF8.GetString(body).Should().Contain("\"org,event\"");
    }

    [Fact]
    public async Task WriteCsvAsync_prefixes_formula_injection_field()
    {
        var dto = MakeDto(eventType: "=CMD(evil)");
        var (_, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(c.Response, "org1", ToAsync([dto]), FixedUtcNow, default));

        Encoding.UTF8.GetString(body).Should().Contain("'=CMD(evil)");
    }

    [Fact]
    public async Task WriteCsvAsync_propagates_cancellation_mid_stream()
    {
        using var cts = new CancellationTokenSource();
        var ctx = new DefaultHttpContext();
        ctx.Response.Body = new MemoryStream();

        await Assert.ThrowsAnyAsync<OperationCanceledException>(() =>
            AuditLogExportStreamer.WriteCsvAsync(
                ctx.Response, "org1",
                CancelAfterNAsync(5, cts),
                FixedUtcNow,
                cts.Token));
    }

    // ── Empty result set edge cases ────────────────────────────────────────────

    [Fact]
    public async Task WriteJsonlAsync_empty_source_writes_headers_only_with_empty_body()
    {
        var (ctx, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteJsonlAsync(
                c.Response, "org1", ToAsync([]), FixedUtcNow, default));

        ctx.Response.ContentType.Should().Be("application/x-ndjson");
        ctx.Response.Headers.ContentDisposition.ToString().Should().StartWith("attachment;");
        body.Should().BeEmpty("JSONL with zero rows has no output beyond the chunked terminator");
    }

    [Fact]
    public async Task WriteCsvAsync_empty_source_writes_header_row_only()
    {
        var (ctx, body) = await CaptureAsync(c =>
            AuditLogExportStreamer.WriteCsvAsync(
                c.Response, "org1", ToAsync([]), FixedUtcNow, default));

        ctx.Response.ContentType.Should().Be("text/csv");
        var text = Encoding.UTF8.GetString(body);
        // Only the header row should be present — no data rows
        var lines = text.TrimEnd('\n').Split('\n');
        lines.Should().HaveCount(1);
        lines[0].Should().Be("created_at,event_type,user_id,user_email,target_type,target_id,success,ip_address");
    }
}
