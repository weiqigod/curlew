// Internal management endpoints for signing key operations.
// Access is restricted to localhost or callers presenting a valid X-Internal-Secret header.
using System.Net;
using System.Security.Cryptography;
using System.Text;
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Maps the <c>/internal/keys</c> endpoint group to the provided route builder.
/// Endpoints are restricted by <see cref="InternalAccessFilter"/>.
/// </summary>
public static class InternalKeysEndpoints
{
    /// <summary>Registers the <c>/internal/keys</c> endpoint group.</summary>
    public static IEndpointRouteBuilder MapInternalKeysEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/internal/keys")
            .AddEndpointFilter<InternalAccessFilter>();

        group.MapGet("/active", async (IKeyProvider kp, CancellationToken ct) =>
        {
            var kid = await kp.GetActiveKidAsync(ct);
            return HttpResults.Json(new { kid });
        });

        group.MapPost("/rotate", async (
            [FromQuery] bool emergency,
            [FromBody] RotateRequest req,
            ISigningKeyRotator rotator,
            CancellationToken ct) =>
        {
            var newKid = await rotator.RotateAsync(emergency, req.Reason, ct);
            return HttpResults.Json(new { kid = newKid });
        });

        return app;
    }
}

/// <summary>Request body for the POST /internal/keys/rotate endpoint.</summary>
internal sealed record RotateRequest(string? Reason);

/// <summary>
/// Endpoint filter that restricts access to loopback connections or requests bearing
/// the <c>X-Internal-Secret</c> header matching <c>APITOOL__INTERNAL__SECRET</c>.
/// Returns 404 (not 403) so external probes cannot fingerprint the endpoint.
/// </summary>
public sealed class InternalAccessFilter : IEndpointFilter
{
    public async ValueTask<object?> InvokeAsync(EndpointFilterInvocationContext ctx, EndpointFilterDelegate next)
    {
        var http = ctx.HttpContext;
        var env = http.RequestServices.GetRequiredService<IWebHostEnvironment>();

        // In Development and Testing environments: allow all requests (localhost ergonomics + tests)
        if (env.IsDevelopment() || env.IsEnvironment("Testing"))
            return await next(ctx);

        var remoteIp = http.Connection.RemoteIpAddress;
        if (remoteIp is not null && IPAddress.IsLoopback(remoteIp))
            return await next(ctx);

        var secret = http.RequestServices.GetRequiredService<IConfiguration>()
            ["ApiTool:Internal:Secret"];
        if (!string.IsNullOrEmpty(secret))
        {
            var headerVal = http.Request.Headers["X-Internal-Secret"].ToString();
            var headerBytes = Encoding.UTF8.GetBytes(headerVal);
            var secretBytes = Encoding.UTF8.GetBytes(secret);
            if (CryptographicOperations.FixedTimeEquals(headerBytes, secretBytes))
                return await next(ctx);
        }

        // Return 404 — don't fingerprint the endpoint to external probers.
        return HttpResults.NotFound();
    }
}
