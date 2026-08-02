using System.Security.Cryptography;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Computes the deterministic <c>deleted-user-{first8(sha256(user_id||org_id))}</c>
/// anonymisation token per spec v4-6. Irreversible — the SHA-256 truncation gives
/// a 4.3B-token space, sufficient for any plausible deployment scale.
/// The token is per-(user, org)-scoped: the same <c>(userId, orgId)</c> pair always
/// derives the same string; different orgs derive different tokens; given just the token,
/// recovering the <c>userId</c> is computationally infeasible.
/// </summary>
public static class AnonymisationToken
{
    /// <summary>Fixed prefix of every anonymisation token.</summary>
    public const string Prefix = "deleted-user-";

    /// <summary>
    /// Returns the per-(user, org) deterministic token.
    /// The <c>userId</c> and <c>orgId</c> bytes are concatenated (16 bytes each, little-endian
    /// .NET GUID layout) and SHA-256 hashed; the first 4 bytes (8 hex chars) are used.
    /// For the user row's own email column, pass <see cref="Guid.Empty"/> as <paramref name="orgId"/>.
    /// </summary>
    public static string Compute(Guid userId, Guid orgId)
    {
        Span<byte> buf = stackalloc byte[32];
        userId.TryWriteBytes(buf[..16]);
        orgId.TryWriteBytes(buf[16..]);
        Span<byte> hash = stackalloc byte[32];
        SHA256.HashData(buf, hash);
        return $"{Prefix}{Convert.ToHexString(hash[..4]).ToLowerInvariant()}";
    }
}
