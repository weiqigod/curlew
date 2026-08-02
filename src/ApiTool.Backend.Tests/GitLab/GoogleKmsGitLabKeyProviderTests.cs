// Tests for GoogleKmsGitLabKeyProvider — KMS-backed DEK wrapping via FakeKmsClient.
// Refs docs/SPECIFICATION.md:9196-9205 (IGitLabKeyProvider envelope encryption).
using System.Security.Cryptography;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Tests for <see cref="GoogleKmsGitLabKeyProvider"/>:
/// round-trip via FakeKmsClient; error surface sanitisation.
/// </summary>
public sealed class GoogleKmsGitLabKeyProviderTests
{
    private const string TestKeyId = "projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1";

    private static IOptions<GitLabOptions> Opts(string kmsKeyId) => Options.Create(new GitLabOptions
    {
        KeyProvider = new GitLabOptions.KeyProviderConfig
        {
            Mode = "kms",
            Kms = new GitLabOptions.KmsConfig { KmsKeyId = kmsKeyId },
        },
    });

    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_a_PAT_through_kms()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsGitLabKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-saas-test");
        var result = await p.EncryptAsync(pt);
        result.Kid.Should().Be(TestKeyId);
        var dec = await p.DecryptAsync(result.Ciphertext, result.Kid);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsGitLabKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("same-plaintext");
        var r1 = await p.EncryptAsync(pt);
        var r2 = await p.EncryptAsync(pt);
        r1.Ciphertext.Should().NotEqual(r2.Ciphertext);
    }

    [Fact]
    public async Task Decrypt_when_kms_throws_surfaces_GitLabPatDecryptException_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsGitLabKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/k/1"));
        // Build a fake valid-looking blob (32 bytes wrapped DEK = 60 bytes of AES-GCM-wrapped, but we
        // only need any blob large enough to pass Unpack's truncation check — the KMS call will fail)
        var dummyBlob = new byte[1 + 2 + 60 + 12 + 1 + 16];
        dummyBlob[0] = 0x01;
        dummyBlob[1] = 0x00;
        dummyBlob[2] = 60;  // wrappedDekLen = 60
        var act = () => p.DecryptAsync(dummyBlob, "projects/secret-project/locations/global/keyRings/kr/cryptoKeys/k/1");
        var ex = (await act.Should().ThrowAsync<GitLabPatDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("GitLab PAT decryption failed (provider=kms)");
    }

    [Fact]
    public async Task Encrypt_when_kms_throws_surfaces_exception_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsGitLabKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/k/1"));
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-saas-test");
        var act = () => p.EncryptAsync(pt);
        var ex = (await act.Should().ThrowAsync<GitLabPatDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("GitLab PAT decryption failed (provider=kms)");
    }

    private sealed class FailingKmsClient : IKmsClient
    {
        public Task<byte[]> AsymmetricSignAsync(string kmsKeyId, byte[] payload, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> AsymmetricSignRsaPkcs1Sha256Async(string kmsKeyId, byte[] payload, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<string> GetPublicKeyJwkAsync(string kmsKeyId, string kid, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> EncryptAsync(string kmsKeyId, byte[] plaintext, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
        public Task<byte[]> DecryptAsync(string kmsKeyId, byte[] ciphertext, CancellationToken ct = default)
            => throw new InvalidOperationException("kms internal");
    }
}
