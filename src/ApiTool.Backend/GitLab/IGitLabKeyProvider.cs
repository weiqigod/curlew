// Refs docs/SPECIFICATION.md:9196-9205 (IGitLabKeyProvider envelope encryption).
namespace ApiTool.Backend.GitLab;

/// <summary>
/// Encrypts and decrypts the GitLab Project Access Token (PAT) for storage in
/// gitlab_installations.access_token_ciphertext. KEK custody differs by deployment
/// (file for self-hosted, KMS for SaaS); per-row DEK pattern bounds blast radius.
/// Refs docs/SPECIFICATION.md:9196-9205 (Encryption — IGitLabKeyProvider).
/// </summary>
public interface IGitLabKeyProvider
{
    /// <summary>
    /// Wraps <paramref name="plaintext"/> into a self-describing AES-256-GCM envelope.
    /// The returned <see cref="GitLabEncryptionResult.Kid"/> identifies the wrapping key
    /// and must be persisted alongside the ciphertext for later decryption.
    /// </summary>
    Task<GitLabEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default);

    /// <summary>
    /// Inverse of <see cref="EncryptAsync"/>. Throws <see cref="GitLabPatDecryptException"/>
    /// when <paramref name="kid"/> does not match the configured KEK or the ciphertext is
    /// corrupt/tampered. Inner exceptions are preserved for telemetry but never appear in
    /// the user-visible message.
    /// </summary>
    Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default);
}

/// <summary>Result of <see cref="IGitLabKeyProvider.EncryptAsync"/>.</summary>
public sealed record GitLabEncryptionResult(byte[] Ciphertext, string Kid);

/// <summary>Thrown when decryption fails; message is provider-kind only (no key material in message).</summary>
public sealed class GitLabPatDecryptException : Exception
{
    /// <summary>Initialises a new exception with a fixed provider-kind message.</summary>
    public GitLabPatDecryptException(string providerKind, Exception? inner = null)
        : base($"GitLab PAT decryption failed (provider={providerKind})", inner) { }
}
