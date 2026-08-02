// Mints single-use tokens for password reset (prst_) and email verification (evtk_).
// Refs docs/SPECIFICATION.md:8493 (prst_ format), :8507 (evtk_ format).
using System.Security.Cryptography;
using System.Text;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Mints opaque single-use tokens for password reset (<c>prst_</c>) and
/// email verification (<c>evtk_</c>). 32 random bytes encoded as base64url
/// give a 43-char body; the prefix gives 5 more for a 48-char total.
/// Refs docs/SPECIFICATION.md:8493 (prst_ format), :8507 (evtk_ format).
/// </summary>
public static class AuthTokenIssuer
{
    /// <summary>The password-reset token prefix (per spec :8493).</summary>
    public const string PasswordResetPrefix = "prst_";

    /// <summary>The email-verification token prefix (per spec :8507).</summary>
    public const string EmailVerificationPrefix = "evtk_";

    /// <summary>
    /// The deletion re-auth token prefix (M18-005).
    /// <c>drto</c> = Deletion Reauth TOken. 5-minute TTL, single-use, API-delivered.
    /// </summary>
    public const string DeletionReauthPrefix = "drto_";

    /// <summary>
    /// Mints a fresh token with the given prefix. Returns the raw plaintext
    /// (sent in the email body) and its SHA-256 hash (stored in the DB).
    /// </summary>
    public static (string Plaintext, byte[] Hash) Mint(string prefix)
    {
        ArgumentException.ThrowIfNullOrEmpty(prefix);
        var rawBytes  = RandomNumberGenerator.GetBytes(32);
        var plaintext = prefix + Base64UrlEncoder.Encode(rawBytes);
        var hash      = Hash(plaintext);
        return (plaintext, hash);
    }

    /// <summary>SHA-256 of the UTF-8 bytes of the full prefixed plaintext.</summary>
    public static byte[] Hash(string plaintext)
        => SHA256.HashData(Encoding.UTF8.GetBytes(plaintext));
}
