// Refs docs/SPECIFICATION.md:8413-8416 (signed-state token for dashboard-initiated install flow).
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Payload carried inside the signed state token for the GitHub App install-URL flow.
/// Encodes org_id, user_id, expiry, and a nonce to prevent identical-payload reuse.
/// </summary>
public sealed record GithubInstallationStatePayload(
    Guid OrgId,
    Guid UserId,
    DateTimeOffset ExpiresAt,
    string Nonce);

/// <summary>
/// Stateless HMAC-SHA256 state token for the GitHub App install-URL flow (spec :8413-8416).
/// Format: Base64Url(JSON body) . Base64Url(HMAC-SHA256(body, key))
/// </summary>
public static class GithubInstallationStateToken
{
    private static readonly JsonSerializerOptions s_json = new(JsonSerializerDefaults.Web)
    {
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    /// <summary>
    /// Mints a signed state token from <paramref name="payload"/> using <paramref name="signingKey"/>.
    /// </summary>
    public static string Mint(GithubInstallationStatePayload payload, string signingKey)
    {
        var bodyJson = JsonSerializer.SerializeToUtf8Bytes(payload, s_json);
        var bodyB64 = Base64UrlEncode(bodyJson);
        var sig = ComputeHmac(bodyB64, signingKey);
        return $"{bodyB64}.{sig}";
    }

    /// <summary>
    /// Verifies <paramref name="token"/> and returns the payload on success;
    /// <see langword="null"/> when the signature mismatches, the token is malformed, or it has expired.
    /// </summary>
    public static GithubInstallationStatePayload? TryVerify(
        string token, string signingKey, DateTimeOffset now)
    {
        var dot = token.IndexOf('.', StringComparison.Ordinal);
        if (dot < 0) return null;

        var bodyB64 = token[..dot];
        var sigB64 = token[(dot + 1)..];

        // Constant-time HMAC comparison
        var expectedSig = ComputeHmac(bodyB64, signingKey);
        if (!CryptographicOperations.FixedTimeEquals(
                Encoding.ASCII.GetBytes(sigB64),
                Encoding.ASCII.GetBytes(expectedSig)))
            return null;

        // Decode body
        byte[] bodyBytes;
        try { bodyBytes = Base64UrlDecode(bodyB64); }
        catch { return null; }

        GithubInstallationStatePayload? payload;
        try { payload = JsonSerializer.Deserialize<GithubInstallationStatePayload>(bodyBytes, s_json); }
        catch { return null; }

        if (payload is null) return null;
        if (now >= payload.ExpiresAt) return null;

        return payload;
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private static string ComputeHmac(string body, string signingKey)
    {
        var keyBytes = Encoding.UTF8.GetBytes(signingKey);
        var bodyBytes = Encoding.ASCII.GetBytes(body);
        var hash = HMACSHA256.HashData(keyBytes, bodyBytes);
        return Base64UrlEncode(hash);
    }

    private static string Base64UrlEncode(ReadOnlySpan<byte> data) =>
        Convert.ToBase64String(data).TrimEnd('=').Replace('+', '-').Replace('/', '_');

    private static byte[] Base64UrlDecode(string s)
    {
        s = s.Replace('-', '+').Replace('_', '/');
        // Add padding
        s += (s.Length % 4) switch
        {
            0 => string.Empty,
            2 => "==",
            3 => "=",
            _ => throw new FormatException("Invalid base64url string"),
        };
        return Convert.FromBase64String(s);
    }
}
