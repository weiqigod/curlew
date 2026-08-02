using System.Security.Cryptography;
using System.Text;

namespace ApiTool.Backend.Invitations;

/// <summary>Generates and hashes secure invitation tokens.</summary>
public static class InvitationTokenGenerator
{
    /// <summary>
    /// Generates a new 48-byte random token encoded as base64url (approximately 64 chars).
    /// </summary>
    /// <returns>The raw token to send to the invitee.</returns>
    public static string GenerateRawToken()
    {
        var bytes = RandomNumberGenerator.GetBytes(48);
        return Convert.ToBase64String(bytes)
            .Replace('+', '-')
            .Replace('/', '_')
            .TrimEnd('=');
    }

    /// <summary>
    /// Computes the SHA-256 hex hash of the given raw token.
    /// Only the hash is stored; the raw token is returned once in the invitation response.
    /// </summary>
    /// <param name="rawToken">The raw token.</param>
    /// <returns>A 64-character lowercase hex string.</returns>
    public static string HashToken(string rawToken)
    {
        var bytes = SHA256.HashData(Encoding.UTF8.GetBytes(rawToken));
        return Convert.ToHexString(bytes).ToLowerInvariant();
    }
}
