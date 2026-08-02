// Generic AES-256-GCM envelope codec shared across all per-row-DEK key providers.
// Wire format is identical to the original GitLabEnvelopeCodec so the binary format is
// unchanged; only the namespace and file location differ.
// Refs docs/SPECIFICATION.md:9196-9205, M18-009 (v4-12).
using System.Security.Cryptography;

namespace ApiTool.Backend.Crypto;

/// <summary>
/// Generic AES-256-GCM envelope codec used by all per-row-DEK key providers
/// (GitLab PAT, team_vaults template, schedules env_vars).
/// Wire format: <c>[0x01][be u16: wrappedDekLen][wrappedDek][12-byte nonce][N bytes ciphertext][16-byte tag]</c>.
/// The split API (GenerateAndEncrypt → Pack, Unpack → DecryptWithDek) lets providers
/// perform async DEK-wrapping (e.g. KMS HTTP call) between the two phases without
/// any sync-over-async blocking inside the codec.
/// Wire format unchanged from the original GitLabEnvelopeCodec.
/// </summary>
public static class EnvelopeCodec
{
    /// <summary>Version byte embedded in every blob; bump when the wire format changes.</summary>
    public const byte Version = 0x01;

    /// <summary>
    /// Phase 1 of encryption: generate a fresh 32-byte DEK and 12-byte nonce,
    /// AES-256-GCM-encrypt <paramref name="plaintext"/>, and return the four components
    /// that the provider must hand to <see cref="Pack"/> after wrapping the DEK.
    /// </summary>
    /// <returns>(dek, nonce, ciphertext, tag) — caller wraps <c>dek</c> with the KEK.</returns>
    public static (byte[] Dek, byte[] Nonce, byte[] Ciphertext, byte[] Tag) GenerateAndEncrypt(byte[] plaintext)
    {
        var dek = RandomNumberGenerator.GetBytes(32);
        var nonce = RandomNumberGenerator.GetBytes(12);
        var ciphertext = new byte[plaintext.Length];
        var tag = new byte[16];
        using (var aes = new AesGcm(dek, tagSizeInBytes: 16))
            aes.Encrypt(nonce, plaintext, ciphertext, tag);
        return (dek, nonce, ciphertext, tag);
    }

    /// <summary>
    /// Phase 2 of encryption: assemble the blob from the wrapped DEK and the GCM outputs.
    /// <paramref name="wrappedDek"/> is the DEK after KEK-wrapping (any length).
    /// </summary>
    public static byte[] Pack(byte[] wrappedDek, byte[] nonce, byte[] ciphertext, byte[] tag)
    {
        if (nonce.Length != 12) throw new ArgumentException("nonce must be 12 bytes", nameof(nonce));
        if (tag.Length != 16) throw new ArgumentException("tag must be 16 bytes", nameof(tag));
        if (wrappedDek.Length > 0xFFFF) throw new ArgumentException("wrappedDek too large", nameof(wrappedDek));

        var blob = new byte[1 + 2 + wrappedDek.Length + 12 + ciphertext.Length + 16];
        blob[0] = Version;
        blob[1] = (byte)(wrappedDek.Length >> 8);
        blob[2] = (byte)(wrappedDek.Length & 0xFF);
        Buffer.BlockCopy(wrappedDek, 0, blob, 3, wrappedDek.Length);
        Buffer.BlockCopy(nonce, 0, blob, 3 + wrappedDek.Length, 12);
        Buffer.BlockCopy(ciphertext, 0, blob, 3 + wrappedDek.Length + 12, ciphertext.Length);
        Buffer.BlockCopy(tag, 0, blob, 3 + wrappedDek.Length + 12 + ciphertext.Length, 16);
        return blob;
    }

    /// <summary>
    /// Phase 1 of decryption: parse the blob and return the four components.
    /// The caller must unwrap <c>wrappedDek</c> with the KEK and pass the raw DEK to
    /// <see cref="DecryptWithDek"/>.
    /// </summary>
    /// <exception cref="CryptographicException">Blob is truncated or has an unsupported version.</exception>
    public static (byte[] WrappedDek, byte[] Nonce, byte[] Ciphertext, byte[] Tag) Unpack(byte[] blob)
    {
        if (blob.Length < 1 + 2 + 12 + 16)
            throw new CryptographicException("envelope: truncated blob");
        if (blob[0] != Version)
            throw new CryptographicException($"envelope: unsupported version 0x{blob[0]:X2}");

        int wrappedDekLen = (blob[1] << 8) | blob[2];
        int hdr = 3 + wrappedDekLen;
        if (blob.Length < hdr + 12 + 16)
            throw new CryptographicException("envelope: truncated (wrappedDek overruns blob)");

        var wrappedDek = new byte[wrappedDekLen];
        Buffer.BlockCopy(blob, 3, wrappedDek, 0, wrappedDekLen);

        var nonce = new byte[12];
        Buffer.BlockCopy(blob, hdr, nonce, 0, 12);

        int ctLen = blob.Length - hdr - 12 - 16;
        var ciphertext = new byte[ctLen];
        Buffer.BlockCopy(blob, hdr + 12, ciphertext, 0, ctLen);

        var tag = new byte[16];
        Buffer.BlockCopy(blob, hdr + 12 + ctLen, tag, 0, 16);

        return (wrappedDek, nonce, ciphertext, tag);
    }

    /// <summary>
    /// Phase 2 of decryption: AES-256-GCM-decrypt using the unwrapped <paramref name="dek"/>.
    /// The DEK is zeroed from memory after use.
    /// </summary>
    /// <exception cref="CryptographicException">GCM authentication fails (corrupt/tampered ciphertext).</exception>
    public static byte[] DecryptWithDek(byte[] dek, byte[] nonce, byte[] ciphertext, byte[] tag)
    {
        try
        {
            var plaintext = new byte[ciphertext.Length];
            using var aes = new AesGcm(dek, tagSizeInBytes: 16);
            aes.Decrypt(nonce, ciphertext, tag, plaintext);
            return plaintext;
        }
        finally
        {
            CryptographicOperations.ZeroMemory(dek);
        }
    }
}
