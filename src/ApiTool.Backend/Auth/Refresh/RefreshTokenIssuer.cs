// Mints opaque refresh tokens: 32 random bytes → base64url plaintext + SHA-256 hash.
// Refs docs/SPECIFICATION.md:7905 (token_hash), :7934 (90/365d lifetime).
using System.Security.Cryptography;
using ApiTool.Backend.Data.Entities;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Pure helper that mints a new opaque refresh token.
/// Returns both the plaintext (sent to the client) and a fully-populated
/// <see cref="RefreshToken"/> row (stored in the database).
/// The plaintext is 32 random bytes encoded as base64url (~43 chars, no padding).
/// The stored hash is SHA-256 of the UTF-8 plaintext bytes.
/// Lifetime follows spec :7934: <c>min(now + 90d, familyRoot.IssuedAt + 365d)</c>.
/// </summary>
public sealed class RefreshTokenIssuer(TimeProvider clock)
{
    /// <summary>
    /// Mints a new opaque refresh token. Returns the plaintext (~43 chars base64url)
    /// and a populated <see cref="RefreshToken"/> row ready for insertion.
    /// </summary>
    public (string Plaintext, RefreshToken Row) Mint(
        Guid userId,
        Guid deviceId,
        Guid familyId,
        Guid? parentId,
        DateTime familyRootIssuedAt,
        string? clientIp,
        string? userAgent)
    {
        var rawBytes  = RandomNumberGenerator.GetBytes(32);
        var plaintext = Base64UrlEncoder.Encode(rawBytes);
        var hash      = Hash(plaintext);

        var now              = clock.GetUtcNow().UtcDateTime;
        var ninetyDayExpiry  = now.AddDays(90);
        var absoluteDeadline = familyRootIssuedAt.AddDays(365);
        var expiresAt        = ninetyDayExpiry < absoluteDeadline ? ninetyDayExpiry : absoluteDeadline;

        var row = new RefreshToken
        {
            Id          = Guid.NewGuid(),
            TokenHash   = hash,
            UserId      = userId,
            DeviceId    = deviceId,
            FamilyId    = familyId,
            ParentId    = parentId,
            IssuedAt    = now,
            ExpiresAt   = expiresAt,
            LastUsedIp  = clientIp,
            UserAgent   = userAgent,
        };

        return (plaintext, row);
    }

    /// <summary>SHA-256 hash of the plaintext token (raw 32 bytes).</summary>
    public static byte[] Hash(string plaintext)
        => SHA256.HashData(System.Text.Encoding.UTF8.GetBytes(plaintext));
}
