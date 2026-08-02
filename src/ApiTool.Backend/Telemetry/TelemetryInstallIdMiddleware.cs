using System.Text.Json;
using Microsoft.AspNetCore.Http;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// ASP.NET Core middleware that pre-reads the body of
/// <c>POST /api/v1/telemetry/events</c> requests to extract the
/// <c>install_id</c> field and stores it in
/// <c>HttpContext.Items["telemetry.install_id"]</c> before the rate-limiter
/// middleware evaluates the partition key.
/// <para>
/// This is required because the ASP.NET Core rate-limiter middleware runs
/// <em>before</em> endpoint filters; a partition key based on the request body
/// cannot be set by an endpoint filter in time. The middleware buffers the body
/// via <see cref="HttpRequest.EnableBuffering"/> so the endpoint handler can
/// re-read it without a second network read.
/// </para>
/// <para>
/// The middleware is registered in the pipeline with
/// <c>app.UseMiddleware&lt;TelemetryInstallIdMiddleware&gt;()</c>
/// immediately before <c>app.UseRateLimiter()</c>.
/// It is a no-op for every path other than the telemetry ingest route.
/// </para>
/// Refs: docs/SPECIFICATION.md — Telemetry Phase 3 Implementation Pipeline.
/// </summary>
public sealed class TelemetryInstallIdMiddleware
{
    private const string TargetPath = "/api/v1/telemetry/events";
    private const string ItemKey = "telemetry.install_id";

    /// <summary>Maximum body bytes to read when peeking for install_id (64 KB cap).</summary>
    private const int MaxPeekBytes = TelemetryIngestEndpoint.MaxBodyBytes;

    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    private readonly RequestDelegate _next;

    /// <summary>Initialises the middleware with the next delegate in the pipeline.</summary>
    public TelemetryInstallIdMiddleware(RequestDelegate next)
    {
        _next = next;
    }

    /// <summary>Invokes the middleware.</summary>
    public async Task InvokeAsync(HttpContext context)
    {
        if (context.Request.Method.Equals("POST", StringComparison.OrdinalIgnoreCase) &&
            context.Request.Path.StartsWithSegments(TargetPath, StringComparison.OrdinalIgnoreCase))
        {
            await PeekInstallIdAsync(context);
        }

        await _next(context);
    }

    private static async Task PeekInstallIdAsync(HttpContext context)
    {
        var request = context.Request;

        // Guard: skip peek if body is obviously over the cap.
        if (request.ContentLength.HasValue && request.ContentLength.Value > MaxPeekBytes)
            return;

        // EnableBuffering allows the endpoint handler to re-read the body after we consume it here.
        request.EnableBuffering();

        try
        {
            using var reader = new StreamReader(request.Body, leaveOpen: true);
            var rawBody = await reader.ReadToEndAsync();

            // Reset so the endpoint handler starts at the beginning.
            request.Body.Position = 0;

            if (rawBody.Length > MaxPeekBytes)
                return;

            // Deserialise only to extract install_id — partial parse is fine here.
            var dto = JsonSerializer.Deserialize<TelemetryIngestRequest>(rawBody, SnakeCaseOptions);
            if (dto?.InstallId is { Length: > 0 } id)
                context.Items[ItemKey] = id;
        }
        catch
        {
            // If parsing fails (malformed JSON, etc.) leave Items[ItemKey] unset.
            // The endpoint handler will return 400; the rate limiter will fall back to "unknown".
            // Reset position on failure so the endpoint can still read the body.
            if (request.Body.CanSeek)
                request.Body.Position = 0;
        }
    }
}
