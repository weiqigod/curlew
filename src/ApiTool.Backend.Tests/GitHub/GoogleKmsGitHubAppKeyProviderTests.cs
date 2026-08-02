using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.Licensing.Keys;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.GitHub;

/// <summary>
/// Tests for GoogleKmsGitHubAppKeyProvider — behavior #2 (KMS dispatch) + error paths.
/// </summary>
public sealed class GoogleKmsGitHubAppKeyProviderTests
{
    private const long TestAppId = 8675309L;

    private static IOptions<GitHubAppOptions> DefaultOpts() => Options.Create(new GitHubAppOptions
    {
        AppId = TestAppId,
        Slug = "apitool-saas",
        KeyProvider = new GitHubAppOptions.KeyProviderConfig
        {
            Mode = "kms",
            Kms = new GitHubAppOptions.KmsConfig
            {
                KmsKeyId = "projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
            },
        },
    });

    [Fact]  // Behavior #2 — KMS dispatch with correct key and verifiable signature
    public async Task SignAppJwtAsync_dispatches_to_kms_RSA_PKCS1_SHA256_with_signing_input()
    {
        using var kms = new FakeKmsClient();
        var provider = new GoogleKmsGitHubAppKeyProvider(kms, DefaultOpts());
        var iat = DateTimeOffset.UtcNow.AddSeconds(-60);
        var jwt = await provider.SignAppJwtAsync(new GitHubAppJwtClaims(TestAppId, iat, iat.AddSeconds(540)));

        kms.RsaSignCalls.Should().HaveCount(1);
        kms.RsaSignCalls[0].KmsKeyId.Should().EndWith("/cryptoKeyVersions/1");

        // The base64url(header) + "." + base64url(payload) is exactly the bytes signed by KMS.
        var parts = jwt.Split('.');
        parts.Should().HaveCount(3);
        var expectedSigningInput = Encoding.ASCII.GetBytes($"{parts[0]}.{parts[1]}");
        kms.RsaSignCalls[0].Payload.Should().Equal(expectedSigningInput);

        // The JWS signature is the base64url-encoded raw signature returned by KMS.
        var sig = Base64UrlEncoder.DecodeBytes(parts[2]);
        var verified = kms.GetLocalRsaPublicKey().VerifyData(expectedSigningInput, sig, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        verified.Should().BeTrue();
    }

    [Fact]
    public async Task SignAppJwtAsync_when_kms_unreachable_throws_GitHubAppJwtSigningException_without_kms_key_in_message()
    {
        var opts = Options.Create(new GitHubAppOptions
        {
            AppId = TestAppId,
            KeyProvider = new GitHubAppOptions.KeyProviderConfig
            {
                Mode = "kms",
                Kms = new GitHubAppOptions.KmsConfig
                {
                    KmsKeyId = "projects/secret-project/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
                },
            },
        });
        var failingKms = new FailingKmsClient();
        var provider = new GoogleKmsGitHubAppKeyProvider(failingKms, opts);
        var claims = new GitHubAppJwtClaims(TestAppId, DateTimeOffset.UtcNow.AddSeconds(-60), DateTimeOffset.UtcNow.AddSeconds(540));

        var act = () => provider.SignAppJwtAsync(claims);

        var ex = (await act.Should().ThrowAsync<GitHubAppJwtSigningException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("GitHub App JWT signing failed (provider=kms)");
    }

    [Fact]
    public async Task SignAppJwtAsync_with_mismatched_app_id_throws_InvalidAppIdMismatch()
    {
        using var kms = new FakeKmsClient();
        var provider = new GoogleKmsGitHubAppKeyProvider(kms, DefaultOpts());
        var act = () => provider.SignAppJwtAsync(new GitHubAppJwtClaims(7777L, DateTimeOffset.UtcNow, DateTimeOffset.UtcNow.AddSeconds(540)));
        await act.Should().ThrowAsync<InvalidAppIdMismatchException>();
    }

    private sealed class FailingKmsClient : IKmsClient
    {
        public Task<byte[]> AsymmetricSignAsync(string kmsKeyId, byte[] payload, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> AsymmetricSignRsaPkcs1Sha256Async(string kmsKeyId, byte[] payload, CancellationToken ct = default)
            => throw new InvalidOperationException("kms unreachable to projects/secret-project/...");
        public Task<string> GetPublicKeyJwkAsync(string kmsKeyId, string kid, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> EncryptAsync(string kmsKeyId, byte[] plaintext, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> DecryptAsync(string kmsKeyId, byte[] ciphertext, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
    }
}
