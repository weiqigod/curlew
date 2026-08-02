// Interface for per-row AES-256-GCM envelope encryption of schedules.env_vars.
// Refs M18-009 (v4-12).
namespace ApiTool.Backend.Schedules.Keys;

/// <summary>
/// Encrypts and decrypts the schedule env_vars JSON for storage in
/// schedules.env_vars_ciphertext. KEK custody differs by deployment
/// (file for self-hosted, KMS for SaaS); per-row DEK pattern bounds blast radius.
/// Refs M18-009 (v4-12).
/// </summary>
public interface IScheduleEnvKeyProvider
{
    /// <summary>
    /// Wraps <paramref name="plaintext"/> into a self-describing AES-256-GCM envelope.
    /// The returned <see cref="ScheduleEnvEncryptionResult.Kid"/> identifies the wrapping key
    /// and must be persisted alongside the ciphertext for later decryption.
    /// </summary>
    Task<ScheduleEnvEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default);

    /// <summary>
    /// Inverse of <see cref="EncryptAsync"/>. Throws <see cref="ScheduleEnvDecryptException"/>
    /// when <paramref name="kid"/> does not match the configured KEK or the ciphertext is
    /// corrupt/tampered. Inner exceptions are preserved for telemetry but never appear in
    /// the user-visible message.
    /// </summary>
    Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default);
}

/// <summary>Result of <see cref="IScheduleEnvKeyProvider.EncryptAsync"/>.</summary>
public sealed record ScheduleEnvEncryptionResult(byte[] Ciphertext, string Kid);

/// <summary>Thrown when decryption fails; message is provider-kind only (no key material in message).</summary>
public sealed class ScheduleEnvDecryptException : Exception
{
    /// <summary>Initialises a new exception with a fixed provider-kind message.</summary>
    public ScheduleEnvDecryptException(string providerKind, Exception? inner = null)
        : base($"schedule env_vars decryption failed (provider={providerKind})", inner) { }
}
