// File-based KEK implementation of IGitLabKeyProvider for self-hosted deployments.
// Refs docs/SPECIFICATION.md:9196-9205 (IGitLabKeyProvider envelope encryption).
using System.Security.Cryptography;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitLab;

/// <summary>
/// <see cref="IGitLabKeyProvider"/> implementation backed by a 32-byte AES-256 KEK file.
/// The KEK wraps a per-row DEK using AES-256-GCM (see <see cref="GitLabEnvelopeCodec"/>).
/// Suitable for self-hosted deployments where a hardware security module is not available.
/// Refs docs/SPECIFICATION.md:9196-9205.
/// </summary>
public sealed class FileGitLabKeyProvider : IGitLabKeyProvider
{
    /// <summary>
    /// Constant kid returned from <see cref="EncryptAsync"/> and expected by <see cref="DecryptAsync"/>.
    /// Bump the version suffix when rotating to a new KEK file (requires re-encryption migration).
    /// </summary>
    public const string KidValue = "gitlab-kek-file-v1";

    private readonly byte[] _kek;  // 32-byte AES-256 KEK

    /// <summary>
    /// Initialises the provider by loading the KEK from disk.
    /// Throws <see cref="InvalidOperationException"/> if the file is missing or not exactly 32 bytes.
    /// </summary>
    public FileGitLabKeyProvider(IOptions<GitLabOptions> opts)
    {
        var path = opts.Value.KeyProvider.File.KekPath;
        if (string.IsNullOrEmpty(path) || !File.Exists(path))
            throw new InvalidOperationException(
                $"GitLab KEK file not found at '{path}'. Set ApiTool:GitLab:KeyProvider:File:KekPath " +
                $"(env GITLAB__KEY_PROVIDER=file GITLAB__KEK_PATH=<path>) to a 32-byte file (mode 0600).");

        var bytes = File.ReadAllBytes(path);
        if (bytes.Length != 32)
            throw new InvalidOperationException(
                $"GitLab KEK file at '{path}' is {bytes.Length} bytes; required size is exactly 32 bytes (AES-256).");

        _kek = bytes;
    }

    /// <inheritdoc/>
    public Task<GitLabEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        var (dek, nonce, ciphertext, tag) = GitLabEnvelopeCodec.GenerateAndEncrypt(plaintext);
        byte[] wrappedDek;
        try
        {
            wrappedDek = WrapDek(dek);
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
        var blob = GitLabEnvelopeCodec.Pack(wrappedDek, nonce, ciphertext, tag);
        return Task.FromResult(new GitLabEncryptionResult(blob, KidValue));
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        if (kid != KidValue)
            throw new GitLabPatDecryptException("file",
                new InvalidOperationException($"kid mismatch: expected '{KidValue}', got '{kid}'"));

        try
        {
            var (wrappedDek, nonce, ct2, tag) = GitLabEnvelopeCodec.Unpack(ciphertext);
            var dek = UnwrapDek(wrappedDek);
            var plaintext = GitLabEnvelopeCodec.DecryptWithDek(dek, nonce, ct2, tag);
            return Task.FromResult(plaintext);
        }
        catch (Exception ex)
        {
            throw new GitLabPatDecryptException("file", ex);
        }
    }

    // ── DEK wrapping / unwrapping using AES-256-GCM under the KEK ────────────────────────

    private byte[] WrapDek(byte[] dek)
    {
        var wrapNonce = RandomNumberGenerator.GetBytes(12);
        var wrappedCt = new byte[dek.Length];
        var wrapTag = new byte[16];
        using var aes = new AesGcm(_kek, tagSizeInBytes: 16);
        aes.Encrypt(wrapNonce, dek, wrappedCt, wrapTag);
        // Wire format: [12 nonce][N wrapped-DEK ciphertext][16 tag]
        var result = new byte[12 + wrappedCt.Length + 16];
        Buffer.BlockCopy(wrapNonce, 0, result, 0, 12);
        Buffer.BlockCopy(wrappedCt, 0, result, 12, wrappedCt.Length);
        Buffer.BlockCopy(wrapTag, 0, result, 12 + wrappedCt.Length, 16);
        return result;
    }

    private byte[] UnwrapDek(byte[] wrapped)
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
        using var aes = new AesGcm(_kek, tagSizeInBytes: 16);
        aes.Decrypt(wrapNonce, wrappedCt, wrapTag, dek);
        return dek;
    }
}
