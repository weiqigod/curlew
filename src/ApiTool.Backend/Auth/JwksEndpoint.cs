// Refs docs/SPECIFICATION.md:7806 (public-key distribution decision),
//      docs/SPECIFICATION.md:8237 (auth surface table — JWKS row),
//      docs/SPECIFICATION.md:8058 (RFC 8615 well-known URI; Cache-Control: public, max-age=3600).
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Maps <c>GET /api/v1/.well-known/jwks.json</c> — the public JWKS endpoint
/// used by the CLI to verify License JWTs and Access tokens offline.
/// Refs docs/SPECIFICATION.md:7806 + :8237 + :8058.
/// </summary>
public static class JwksEndpoint
{
    /// <summary>Cache lifetime advertised by the response (RFC 8615 well-known URI convention).</summary>
    public const int CacheMaxAgeSeconds = 3600;

    /// <summary>Registers <c>GET /api/v1/.well-known/jwks.json</c>.</summary>
    public static IEndpointRouteBuilder MapJwksEndpoint(this IEndpointRouteBuilder app)
    {
        app.MapGet("/api/v1/.well-known/jwks.json", async (
            IKeyProvider keys,
            HttpContext http,
            CancellationToken ct) =>
        {
            // Force-bootstrap a current key if none exists so the JWKS is never
            // empty in steady state (plan Decision #8).
            _ = await keys.GetActiveKidAsync(ct);

            var jwks = await keys.GetVerificationJwksAsync(ct);

            http.Response.Headers.CacheControl = $"public, max-age={CacheMaxAgeSeconds}";

            // Serialise to the canonical {"keys":[...]} JWKS shape.
            var json = JsonSerializer.Serialize(jwks);
            return HttpResults.Content(json, "application/jwk-set+json");
        })
        .AllowAnonymous()
        .WithName("AuthJwks")
        .WithTags("Auth")
        .Produces<System.Text.Json.JsonElement>(StatusCodes.Status200OK, contentType: "application/jwk-set+json");

        return app;
    }
}
