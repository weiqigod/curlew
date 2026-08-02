// Refs docs/SPECIFICATION.md:8377-8385 (App JWT shape — alg/typ/iss/iat/exp fixed by GitHub).
// GHES is NOT supported (:8696); api.github.com is hardcoded by token-exchange callers.
using System.Text;
using System.Text.Json;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// Builds the unsigned JWS input and assembles the final compact JWS for GitHub App JWTs.
/// Internal helper shared by both file and KMS provider implementations.
/// </summary>
internal static class AppJwtBuilder
{
    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNamingPolicy = null,  // raw lower-case keys; GitHub is case-sensitive
    };

    /// <summary>
    /// Builds the unsigned JWS signing input — base64url(header) + "." + base64url(payload).
    /// Returns the input bytes plus the encoded header/payload for re-use when assembling the JWS.
    /// </summary>
    public static (byte[] SigningInput, string HeaderB64, string PayloadB64) BuildUnsigned(GitHubAppJwtClaims claims)
    {
        // Header — GitHub mandates RS256 and JWT; no kid (GitHub does not require it for App JWTs)
        var header = new SortedDictionary<string, object>
        {
            ["alg"] = "RS256",
            ["typ"] = "JWT",
        };
        var headerB64 = Base64UrlEncoder.Encode(
            Encoding.UTF8.GetBytes(JsonSerializer.Serialize(header, JsonOpts)));

        var payload = new SortedDictionary<string, object>
        {
            ["exp"] = claims.Exp.ToUnixTimeSeconds(),
            ["iat"] = claims.Iat.ToUnixTimeSeconds(),
            ["iss"] = claims.AppId,
        };
        var payloadB64 = Base64UrlEncoder.Encode(
            Encoding.UTF8.GetBytes(JsonSerializer.Serialize(payload, JsonOpts)));

        var signingInput = Encoding.ASCII.GetBytes($"{headerB64}.{payloadB64}");
        return (signingInput, headerB64, payloadB64);
    }

    /// <summary>Assembles the compact JWS from encoded header/payload and raw signature bytes.</summary>
    public static string Assemble(string headerB64, string payloadB64, byte[] signature)
        => $"{headerB64}.{payloadB64}.{Base64UrlEncoder.Encode(signature)}";
}
