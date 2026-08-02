// Interface for per-row AES-256-GCM envelope encryption of team_vaults.template_jsonb.
// Refs M18-009 (v4-12).
namespace ApiTool.Backend.VaultConfig.Keys;

/// <summary>
/// Encrypts and decrypts the team vault template for storage in
/// team_vaults.template_jsonb_ciphertext. KEK custody differs by deployment
/// (file for self-hosted, KMS for SaaS); per-row DEK pattern bounds blast radius.
/// Refs M18-009 (v4-12).
/// </summary>
public interface ITeamVaultKeyProvider
{
    /// <summary>
    /// Wraps <paramref name="plaintext"/> into a self-describing AES-256-GCM envelope.
    /// The returned <see cref="TeamVaultEncryptionResult.Kid"/> identifies the wrapping key
    /// and must be persisted alongside the ciphertext for later decryption.
    /// </summary>
    Task<TeamVaultEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default);

    /// <summary>
    /// Inverse of <see cref="EncryptAsync"/>. Throws <see cref="TeamVaultDecryptException"/>
    /// when <paramref name="kid"/> does not match the configured KEK or the ciphertext is
    /// corrupt/tampered. Inner exceptions are preserved for telemetry but never appear in
    /// the user-visible message.
    /// </summary>
    Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default);
}

/// <summary>Result of <see cref="ITeamVaultKeyProvider.EncryptAsync"/>.</summary>
public sealed record TeamVaultEncryptionResult(byte[] Ciphertext, string Kid);

/// <summary>Thrown when decryption fails; message is provider-kind only (no key material in message).</summary>
public sealed class TeamVaultDecryptException : Exception
{
    /// <summary>Initialises a new exception with a fixed provider-kind message.</summary>
    public TeamVaultDecryptException(string providerKind, Exception? inner = null)
        : base($"team-vault template decryption failed (provider={providerKind})", inner) { }
}
