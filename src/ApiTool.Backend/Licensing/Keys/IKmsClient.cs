namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Minimal abstraction over the Google Cloud KMS API for asymmetric signing operations.
/// Injected into <see cref="GoogleKmsKeyProvider"/>; in tests replaced by a
/// <c>FakeKmsClient</c> to avoid live GCP calls (hard rule: no real-world cost).
/// </summary>
public interface IKmsClient
{
    /// <summary>
    /// Signs <paramref name="payload"/> with an EC P-256 KMS key (used by the License-JWT pipeline).
    /// Returns raw R||S 64-byte signature (EC_SIGN_P256_SHA256).
    /// </summary>
    Task<byte[]> AsymmetricSignAsync(string kmsKeyId, byte[] payload, CancellationToken ct = default);

    /// <summary>
    /// Signs <paramref name="payload"/> with an <c>RSA_SIGN_PKCS1_2048_SHA256</c> KMS key
    /// (used by the GitHub App JWT pipeline — RS256 mandated by GitHub).
    /// Returns the raw 256-byte PKCS#1 v1.5 RSA signature suitable for direct use as the JWS signature.
    /// Refs docs/SPECIFICATION.md:8355.
    /// </summary>
    Task<byte[]> AsymmetricSignRsaPkcs1Sha256Async(string kmsKeyId, byte[] payload, CancellationToken ct = default);

    /// <summary>
    /// Retrieves the public key for <paramref name="kmsKeyId"/> and returns it serialised as a
    /// JWK JSON string with <c>kid</c> set to <paramref name="kid"/>.
    /// </summary>
    Task<string> GetPublicKeyJwkAsync(string kmsKeyId, string kid, CancellationToken ct = default);

    /// <summary>
    /// Symmetric envelope encryption against an AES-256 KMS key (used to wrap GitLab PAT DEKs).
    /// Returns opaque KMS-defined ciphertext suitable for storage and later passed to
    /// <see cref="DecryptAsync"/> with the same <paramref name="kmsKeyId"/>.
    /// Refs docs/SPECIFICATION.md (IGitLabKeyProvider envelope encryption).
    /// </summary>
    Task<byte[]> EncryptAsync(string kmsKeyId, byte[] plaintext, CancellationToken ct = default);

    /// <summary>
    /// Inverse of <see cref="EncryptAsync"/>. Returns the original plaintext.
    /// Throws if the ciphertext was produced by a different key or is corrupt.
    /// </summary>
    Task<byte[]> DecryptAsync(string kmsKeyId, byte[] ciphertext, CancellationToken ct = default);
}
