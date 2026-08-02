// KMS-backed IGitLabKeyProvider for SaaS deployments (HSM-tier key custody).
// Refs docs/SPECIFICATION.md:9196-9205 (IGitLabKeyProvider envelope encryption).
using System.Security.Cryptography;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitLab;

/// <summary>
/// <see cref="IGitLabKeyProvider"/> implementation that wraps per-row DEKs using Google Cloud KMS.
/// The KMS key ID acts as the <c>kid</c> so the row records which KMS key version produced the
/// ciphertext, enabling key rotation without bulk re-encryption.
/// Refs docs/SPECIFICATION.md:9196-9205.
/// </summary>
public sealed class GoogleKmsGitLabKeyProvider(IKmsClient kms, IOptions<GitLabOptions> opts) : IGitLabKeyProvider
{
    private readonly string _kmsKeyId = opts.Value.KeyProvider.Kms.KmsKeyId;

    /// <inheritdoc/>
    public async Task<GitLabEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
    {
        try
        {
            var (dek, nonce, ciphertext, tag) = GitLabEnvelopeCodec.GenerateAndEncrypt(plaintext);
            byte[] wrappedDek;
            try
            {
                wrappedDek = await kms.EncryptAsync(_kmsKeyId, dek, ct);
            }
            finally
            {
                CryptographicOperations.ZeroMemory(dek);
            }
            var blob = GitLabEnvelopeCodec.Pack(wrappedDek, nonce, ciphertext, tag);
            return new GitLabEncryptionResult(blob, _kmsKeyId);
        }
        catch (GitLabPatDecryptException)
        {
            throw;  // don't double-wrap
        }
        catch (Exception ex)
        {
            // NOTE: GitLabPatDecryptException is reused here for encrypt failures to avoid
            // proliferating exception types. The message says "decryption failed" but the
            // context is an encrypt call. If this causes operator confusion a rename to
            // GitLabPatCryptoException covering both directions is the correct follow-up.
            throw new GitLabPatDecryptException("kms", ex);
        }
    }

    /// <inheritdoc/>
    public async Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
    {
        try
        {
            var (wrappedDek, nonce, ct2, tag) = GitLabEnvelopeCodec.Unpack(ciphertext);
            var dek = await kms.DecryptAsync(kid, wrappedDek, ct);
            return GitLabEnvelopeCodec.DecryptWithDek(dek, nonce, ct2, tag);
        }
        catch (GitLabPatDecryptException)
        {
            throw;  // don't double-wrap
        }
        catch (Exception ex)
        {
            throw new GitLabPatDecryptException("kms", ex);
        }
    }
}
