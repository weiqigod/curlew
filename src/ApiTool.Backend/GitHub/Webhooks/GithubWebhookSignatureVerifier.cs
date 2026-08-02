// Refs docs/SPECIFICATION.md:8568-8575 (signature verification, SHA-1 rejection, multi-secret rotation).
using System.Security.Cryptography;
using System.Text;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>HMAC-SHA256 verifier for GitHub webhook deliveries.</summary>
public static class GithubWebhookSignatureVerifier
{
    private const string Sha256Prefix = "sha256=";

    /// <summary>
    /// Verifies <paramref name="signatureHeader"/> against <paramref name="rawBody"/> using
    /// constant-time comparison against each secret in <paramref name="secrets"/>. Succeeds
    /// on any match (multi-secret rotation). Returns false when the header is malformed
    /// (wrong prefix, non-hex, wrong length) or no secret matches. Constant-time per spec :8572.
    /// </summary>
    public static bool TryVerify(byte[] rawBody, string? signatureHeader, IReadOnlyList<string> secrets)
    {
        if (string.IsNullOrEmpty(signatureHeader)) return false;
        if (!signatureHeader.StartsWith(Sha256Prefix, StringComparison.Ordinal)) return false;
        var hex = signatureHeader.AsSpan(Sha256Prefix.Length);
        if (hex.Length != 64) return false; // 32-byte HMAC-SHA256 = 64 hex chars

        Span<byte> received = stackalloc byte[32];
        if (!TryHexDecode(hex, received)) return false;

        Span<byte> computed = stackalloc byte[32];
        foreach (var secret in secrets)
        {
            if (string.IsNullOrEmpty(secret)) continue;
            using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
            if (!hmac.TryComputeHash(rawBody, computed, out _)) continue;
            if (CryptographicOperations.FixedTimeEquals(received, computed)) return true;
        }
        return false;
    }

    private static bool TryHexDecode(ReadOnlySpan<char> hex, Span<byte> output)
    {
        if (hex.Length != output.Length * 2) return false;
        for (var i = 0; i < output.Length; i++)
        {
            var hi = HexNibble(hex[i * 2]);
            var lo = HexNibble(hex[i * 2 + 1]);
            if (hi < 0 || lo < 0) return false;
            output[i] = (byte)((hi << 4) | lo);
        }
        return true;
    }

    private static int HexNibble(char c) => c switch
    {
        >= '0' and <= '9' => c - '0',
        >= 'a' and <= 'f' => c - 'a' + 10,
        >= 'A' and <= 'F' => c - 'A' + 10,
        _ => -1,
    };
}
