// Refs docs/SPECIFICATION.md:8350-8388. RS256 only; RSA_SIGN_PKCS1_2048_SHA256 KMS key (HSM tier).
// GHES is NOT supported (:8696); api.github.com is hardcoded by callers.
using ApiTool.Backend.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// Google KMS-backed <see cref="IGitHubAppKeyProvider"/> for SaaS deployments.
/// Dispatches signing to an <c>RSA_SIGN_PKCS1_2048_SHA256</c> KMS key (HSM tier).
/// Key material never leaves KMS; only the signed bytes are returned.
/// Refs docs/SPECIFICATION.md:8350-8388.
/// </summary>
public sealed class GoogleKmsGitHubAppKeyProvider(
    IKmsClient kms,
    IOptions<GitHubAppOptions> opts) : IGitHubAppKeyProvider
{
    private readonly GitHubAppOptions _opts = opts.Value;

    /// <inheritdoc/>
    public long GetAppId() => _opts.AppId;

    /// <inheritdoc/>
    public async Task<string> SignAppJwtAsync(GitHubAppJwtClaims claims, CancellationToken ct = default)
    {
        if (claims.AppId != _opts.AppId)
            throw new InvalidAppIdMismatchException(claims.AppId, _opts.AppId);

        var (signingInput, headerB64, payloadB64) = AppJwtBuilder.BuildUnsigned(claims);

        try
        {
            var signature = await kms.AsymmetricSignRsaPkcs1Sha256Async(
                _opts.KeyProvider.Kms.KmsKeyId, signingInput, ct);
            return AppJwtBuilder.Assemble(headerB64, payloadB64, signature);
        }
        catch (Exception ex) when (ex is not InvalidAppIdMismatchException)
        {
            throw new GitHubAppJwtSigningException("kms", ex);
        }
    }
}
