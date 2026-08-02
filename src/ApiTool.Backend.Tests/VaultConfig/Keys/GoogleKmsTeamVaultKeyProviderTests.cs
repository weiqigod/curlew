// Tests for GoogleKmsTeamVaultKeyProvider — KMS-backed DEK wrapping via FakeKmsClient.
// Refs M18-009 (v4-12).
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.Licensing.Keys;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.VaultConfig.Keys;

/// <summary>
/// Tests for <see cref="GoogleKmsTeamVaultKeyProvider"/>:
/// round-trip via FakeKmsClient; error surface sanitisation.
/// </summary>
public sealed class GoogleKmsTeamVaultKeyProviderTests
{
    private const string TestKeyId = "projects/p/locations/global/keyRings/kr/cryptoKeys/teamvault/cryptoKeyVersions/1";

    private static IOptions<TeamVaultEncryptionOptions> Opts(string kmsKeyId) => Options.Create(new TeamVaultEncryptionOptions
    {
        KeyProvider = new TeamVaultEncryptionOptions.KeyProviderConfig
        {
            Mode = "kms",
            Kms = new TeamVaultEncryptionOptions.KmsConfig { KmsKeyId = kmsKeyId },
        },
    });

    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_a_template_through_kms()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsTeamVaultKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("team_secrets:\n  provider: aws-secrets-manager\n");
        var result = await p.EncryptAsync(pt);
        result.Kid.Should().Be(TestKeyId);
        var dec = await p.DecryptAsync(result.Ciphertext, result.Kid);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsTeamVaultKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("same-plaintext");
        var r1 = await p.EncryptAsync(pt);
        var r2 = await p.EncryptAsync(pt);
        r1.Ciphertext.Should().NotEqual(r2.Ciphertext);
    }

    [Fact]
    public async Task Decrypt_when_kms_throws_surfaces_TeamVaultDecryptException_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsTeamVaultKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/teamvault/1"));
        // Build a valid-looking blob large enough to pass Unpack's truncation check — the KMS call will fail.
        var dummyBlob = new byte[1 + 2 + 60 + 12 + 1 + 16];
        dummyBlob[0] = 0x01;
        dummyBlob[1] = 0x00;
        dummyBlob[2] = 60;  // wrappedDekLen = 60
        var act = () => p.DecryptAsync(dummyBlob, "projects/secret-project/locations/global/keyRings/kr/cryptoKeys/teamvault/1");
        var ex = (await act.Should().ThrowAsync<TeamVaultDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("team-vault template decryption failed (provider=kms)");
    }

    [Fact]
    public async Task Encrypt_when_kms_throws_surfaces_exception_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsTeamVaultKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/teamvault/1"));
        var pt = System.Text.Encoding.UTF8.GetBytes("template-payload");
        var act = () => p.EncryptAsync(pt);
        var ex = (await act.Should().ThrowAsync<TeamVaultDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("team-vault template decryption failed (provider=kms)");
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
