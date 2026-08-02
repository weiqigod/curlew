// KMS-backed ITeamVaultKeyProvider for SaaS deployments (HSM-tier key custody).
// Refs M18-009 (v4-12).
using System.Security.Cryptography;
using ApiTool.Backend.Crypto;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.VaultConfig.Keys;

/// <summary>
/// <see cref="ITeamVaultKeyProvider"/> implementation that wraps per-row DEKs using Google Cloud KMS.
/// The KMS key ID acts as the <c>kid</c> so the row records which KMS key version produced the
/// ciphertext, enabling key rotation without bulk re-encryption.
/// Refs M18-009 (v4-12).
/// </summary>
public sealed class GoogleKmsTeamVaultKeyProvider(IKmsClient kms, IOptions<TeamVaultEncryptionOptions> opts) : ITeamVaultKeyProvider
{
    private readonly string _kmsKeyId = opts.Value.KeyProvider.Kms.KmsKeyId;

    /// <inheritdoc/>
    public async Task<TeamVaultEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        try
        {
            var (dek, nonce, ciphertext, tag) = EnvelopeCodec.GenerateAndEncrypt(plaintext);
            byte[] wrappedDek;
            try
            {
                wrappedDek = await kms.EncryptAsync(_kmsKeyId, dek, ct);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(dek);
            }
            var blob = EnvelopeCodec.Pack(wrappedDek, nonce, ciphertext, tag);
            return new TeamVaultEncryptionResult(blob, _kmsKeyId);
        }
        catch (TeamVaultDecryptException)
        {
            throw;  // don't double-wrap
        }
        catch (Exception ex)
        {
            throw new TeamVaultDecryptException("kms", ex);
        }
    }

    /// <inheritdoc/>
    public async Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        try
        {
            var (wrappedDek, nonce, ct2, tag) = EnvelopeCodec.Unpack(ciphertext);
            var dek = await kms.DecryptAsync(kid, wrappedDek, ct);
            return EnvelopeCodec.DecryptWithDek(dek, nonce, ct2, tag);
        }
        catch (TeamVaultDecryptException)
        {
            throw;  // don't double-wrap
        }
        catch (Exception ex)
        {
            throw new TeamVaultDecryptException("kms", ex);
        }
    }
}
