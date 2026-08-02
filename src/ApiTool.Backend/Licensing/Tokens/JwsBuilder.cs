// Internal helper: builds a compact JWS string by signing header+payload via IKeyProvider.
// Algorithm is always ES256 — no other algorithm is permitted.
// Refs docs/SPECIFICATION.md:7992, RFC 8725 §3.11.
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Builds compact JWS tokens (header.payload.signature) using <see cref="IKeyProvider"/>
/// for signing. Algorithm is pinned to ES256 (RFC 8725 §3.11); no runtime indirection.
/// </summary>
internal static class JwsBuilder
{
    // Headers may omit null values; payloads serialize null claims explicitly (trial_expiry, org_id, etc.)
    private static readonly JsonSerializerOptions HeaderJsonOpts = new()
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    };

    private static readonly JsonSerializerOptions PayloadJsonOpts = new()
    {
        // Null claims MUST appear in the payload (e.g., trial_expiry, org_role) —
        // do NOT skip them; consumers rely on their presence for schema-stability.
        DefaultIgnoreCondition = JsonIgnoreCondition.Never,
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
    };

    /// <summary>
    /// Builds a compact JWS by signing header+payload via <see cref="IKeyProvider.SignAsync"/>.
    /// </summary>
    /// <param name="keyProvider">The active key provider for signing.</param>
    /// <param name="headerClaims">
    ///   Extra header entries — <c>typ</c> and <c>kid</c>. <c>alg</c> is always forced to <c>ES256</c>.
    /// </param>
    /// <param name="payloadClaims">The JWT payload claims (string keys, object? values).</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>A compact JWS string: <c>header.payload.signature</c>.</returns>
    public static async Task<string> BuildAsync(
        IKeyProvider keyProvider,
        IReadOnlyDictionary<string, object?> headerClaims,
        IReadOnlyDictionary<string, object?> payloadClaims,
        CancellationToken ct)
    {
        // We need the kid before constructing the header, so fetch the active kid first,
        // then sign. SignAsync internally fetches the same current key.
        var activeClaims = new Dictionary<string, object?>(headerClaims)
        {
            ["alg"] = "ES256",
        };

        var headerJson = JsonSerializer.Serialize(activeClaims, HeaderJsonOpts);
        var headerB64 = Base64UrlEncoder.Encode(Encoding.UTF8.GetBytes(headerJson));

        var payloadJson = JsonSerializer.Serialize(payloadClaims, PayloadJsonOpts);
        var payloadB64 = Base64UrlEncoder.Encode(Encoding.UTF8.GetBytes(payloadJson));

        var signingInput = Encoding.ASCII.GetBytes($"{headerB64}.{payloadB64}");
        var result = await keyProvider.SignAsync(signingInput, ct);

        var signatureB64 = Base64UrlEncoder.Encode(result.Signature);
        return $"{headerB64}.{payloadB64}.{signatureB64}";
    }
}
