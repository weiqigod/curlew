// File-based KEK implementation of IScheduleEnvKeyProvider for self-hosted deployments.
// Refs M18-009 (v4-12).
using System.Security.Cryptography;
using ApiTool.Backend.Crypto;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Schedules.Keys;

/// <summary>
/// <see cref="IScheduleEnvKeyProvider"/> implementation backed by a 32-byte AES-256 KEK file.
/// The KEK wraps a per-row DEK using AES-256-GCM (see <see cref="EnvelopeCodec"/>).
/// Suitable for self-hosted deployments where a hardware security module is not available.
/// Refs M18-009 (v4-12).
/// </summary>
public sealed class FileScheduleEnvKeyProvider : IScheduleEnvKeyProvider
{
    /// <summary>
    /// Constant kid returned from <see cref="EncryptAsync"/> and expected by <see cref="DecryptAsync"/>.
    /// Bump the version suffix when rotating to a new KEK file (requires re-encryption migration).
    /// </summary>
    public const string KidValue = "schedule-env-kek-file-v1";

    private readonly string _kekPath;
    private byte[]? _kekCache;

    /// <summary>
    /// Initialises the provider. The KEK file is loaded lazily on first use so that
    /// non-schedule code paths (e.g., health checks, internal endpoints) can start without
    /// a configured KEK file — only encrypt/decrypt calls will fail at runtime if the path
    /// is missing.
    /// </summary>
    public FileScheduleEnvKeyProvider(IOptions<ScheduleEnvEncryptionOptions> opts)
    {
        _kekPath = opts.Value.KeyProvider.File.KekPath;
    }

    /// <inheritdoc/>
    public Task<ScheduleEnvEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        var kek = LoadKek();
        var (dek, nonce, ciphertext, tag) = EnvelopeCodec.GenerateAndEncrypt(plaintext);
        byte[] wrappedDek;
        try
        {
            wrappedDek = WrapDek(kek, dek);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
        var blob = EnvelopeCodec.Pack(wrappedDek, nonce, ciphertext, tag);
        return Task.FromResult(new ScheduleEnvEncryptionResult(blob, KidValue));
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        if (kid != KidValue)
            throw new ScheduleEnvDecryptException("file",
                new InvalidOperationException($"kid mismatch: expected '{KidValue}', got '{kid}'"));

        var kek = LoadKek();
        try
        {
            var (wrappedDek, nonce, ct2, tag) = EnvelopeCodec.Unpack(ciphertext);
            var dek = UnwrapDek(kek, wrappedDek);
            var plaintext = EnvelopeCodec.DecryptWithDek(dek, nonce, ct2, tag);
            return Task.FromResult(plaintext);
        }
        catch (ScheduleEnvDecryptException)
        {
            throw;
        }
        catch (Exception ex)
        {
            throw new ScheduleEnvDecryptException("file", ex);
        }
    }

    // ── KEK lazy load ─────────────────────────────────────────────────────────

    private byte[] LoadKek()
    {
        if (_kekCache is { } cached)
            return cached;

        if (string.IsNullOrEmpty(_kekPath) || !File.Exists(_kekPath))
            throw new InvalidOperationException(
                $"ScheduleEnv KEK file not found at '{_kekPath}'. Set ApiTool:Schedules:Encryption:KeyProvider:File:KekPath " +
                $"(env SCHEDULES__KEY_PROVIDER=file SCHEDULES__KEK_PATH=<path>) to a 32-byte file (mode 0600).");

        var bytes = File.ReadAllBytes(_kekPath);
        if (bytes.Length != 32)
            throw new InvalidOperationException(
                $"ScheduleEnv KEK file at '{_kekPath}' is {bytes.Length} bytes; required size is exactly 32 bytes (AES-256).");

        return _kekCache = bytes;
    }

    // ── DEK wrapping / unwrapping using AES-256-GCM under the KEK ────────────────────────

    private static byte[] WrapDek(byte[] kek, byte[] dek)
    {
        var wrapNonce = RandomNumberGenerator.GetBytes(12);
        var wrappedCt = new byte[dek.Length];
        var wrapTag = new byte[16];
        using var aes = new AesGcm(kek, tagSizeInBytes: 16);
        aes.Encrypt(wrapNonce, dek, wrappedCt, wrapTag);
        // Wire format: [12 nonce][N wrapped-DEK ciphertext][16 tag]
        var result = new byte[12 + wrappedCt.Length + 16];
        Buffer.BlockCopy(wrapNonce, 0, result, 0, 12);
        Buffer.BlockCopy(wrappedCt, 0, result, 12, wrappedCt.Length);
        Buffer.BlockCopy(wrapTag, 0, result, 12 + wrappedCt.Length, 16);
        return result;
    }

    private static byte[] UnwrapDek(byte[] kek, byte[] wrapped)
    {
        if (wrapped.Length < 12 + 16)
            throw new CryptographicException("wrapped DEK truncated");

        var wrapNonce = new byte[12];
        var wrapTag = new byte[16];
        int ctLen = wrapped.Length - 12 - 16;
        var wrappedCt = new byte[ctLen];
        Buffer.BlockCopy(wrapped, 0, wrapNonce, 0, 12);
        Buffer.BlockCopy(wrapped, 12, wrappedCt, 0, ctLen);
        Buffer.BlockCopy(wrapped, 12 + ctLen, wrapTag, 0, 16);

        var dek = new byte[ctLen];
        using var aes = new AesGcm(kek, tagSizeInBytes: 16);
        aes.Decrypt(wrapNonce, wrappedCt, wrapTag, dek);
        return dek;
    }
}
