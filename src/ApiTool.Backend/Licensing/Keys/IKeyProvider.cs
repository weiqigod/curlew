// Refs docs/SPECIFICATION.md:8048-8054 (IKeyProvider interface).
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Signing-key abstraction used by the token-issuance pipeline.
/// Refs docs/SPECIFICATION.md:8048-8054.
/// </summary>
public interface IKeyProvider
{
    /// <summary>
    /// Signs <paramref name="payload"/> with the active signing key.
    /// Returns a <see cref="SignatureResult"/> containing the kid and the raw
    /// signature bytes (R||S, 64 bytes for ES256).
    /// </summary>
    Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default);

    /// <summary>
    /// Returns the kid of the key currently in <c>status='current'</c>.
    /// Boots a new key if none exists.
    /// </summary>
    Task<string> GetActiveKidAsync(CancellationToken ct = default);

    /// <summary>
    /// Returns a JWKS containing the current key plus all keys in the verification window
    /// (status = 'verifying').
    /// </summary>
    Task<JsonWebKeySet> GetVerificationJwksAsync(CancellationToken ct = default);

}

/// <summary>
/// Carries the kid and the raw ECDSA signature bytes (R||S 64 bytes for ES256).
/// </summary>
public readonly record struct SignatureResult(string Kid, byte[] Signature);
