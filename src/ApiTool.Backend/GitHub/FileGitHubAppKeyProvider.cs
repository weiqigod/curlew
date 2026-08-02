// Refs docs/SPECIFICATION.md:8350-8388. GHES is NOT supported (:8696);
// api.github.com is hardcoded by callers (this provider is purely a signing primitive).
using System.Security.Cryptography;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// File-system-backed <see cref="IGitHubAppKeyProvider"/> for self-hosted deployments.
/// Reads the App private key from <see cref="GitHubAppOptions.KeyProviderConfig.File"/>
/// (PKCS#8 or PKCS#1 PEM). Mode 0600 is enforced on Unix at startup; PEM contents
/// are NEVER logged or included in exception messages.
/// Refs docs/SPECIFICATION.md:8350-8388.
/// </summary>
public sealed class FileGitHubAppKeyProvider(IOptions<GitHubAppOptions> opts) : IGitHubAppKeyProvider
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
            var pem = await File.ReadAllTextAsync(_opts.KeyProvider.File.Path, ct);
            using var rsa = RSA.Create();
            rsa.ImportFromPem(pem);
            var signature = rsa.SignData(signingInput, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
            return AppJwtBuilder.Assemble(headerB64, payloadB64, signature);
        }
        catch (Exception ex) when (ex is not InvalidAppIdMismatchException)
        {
            throw new GitHubAppJwtSigningException("file", ex);
        }
    }
}
