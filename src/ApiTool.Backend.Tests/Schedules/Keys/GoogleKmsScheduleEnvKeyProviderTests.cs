// Tests for GoogleKmsScheduleEnvKeyProvider — KMS-backed DEK wrapping via FakeKmsClient.
// Refs M18-009 (v4-12).
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Schedules.Keys;
using ApiTool.Backend.Tests.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Schedules.Keys;

/// <summary>
/// Tests for <see cref="GoogleKmsScheduleEnvKeyProvider"/>:
/// round-trip via FakeKmsClient; error surface sanitisation.
/// </summary>
public sealed class GoogleKmsScheduleEnvKeyProviderTests
{
    private const string TestKeyId = "projects/p/locations/global/keyRings/kr/cryptoKeys/schedule-env/cryptoKeyVersions/1";

    private static IOptions<ScheduleEnvEncryptionOptions> Opts(string kmsKeyId) => Options.Create(new ScheduleEnvEncryptionOptions
    {
        KeyProvider = new ScheduleEnvEncryptionOptions.KeyProviderConfig
        {
            Mode = "kms",
            Kms = new ScheduleEnvEncryptionOptions.KmsConfig { KmsKeyId = kmsKeyId },
        },
    });

    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_env_vars_through_kms()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsScheduleEnvKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("{\"API_KEY\":\"saas-secret\",\"REGION\":\"us-east-1\"}");
        var result = await p.EncryptAsync(pt);
        result.Kid.Should().Be(TestKeyId);
        var dec = await p.DecryptAsync(result.Ciphertext, result.Kid);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call()
    {
        using var kms = new FakeKmsClient();
        var p = new GoogleKmsScheduleEnvKeyProvider(kms, Opts(TestKeyId));
        var pt = System.Text.Encoding.UTF8.GetBytes("{\"KEY\":\"same\"}");
        var r1 = await p.EncryptAsync(pt);
        var r2 = await p.EncryptAsync(pt);
        r1.Ciphertext.Should().NotEqual(r2.Ciphertext);
    }

    [Fact]
    public async Task Decrypt_when_kms_throws_surfaces_ScheduleEnvDecryptException_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsScheduleEnvKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/se/1"));
        var dummyBlob = new byte[1 + 2 + 60 + 12 + 1 + 16];
        dummyBlob[0] = 0x01;
        dummyBlob[1] = 0x00;
        dummyBlob[2] = 60;
        var act = () => p.DecryptAsync(dummyBlob, "projects/secret-project/locations/global/keyRings/kr/cryptoKeys/se/1");
        var ex = (await act.Should().ThrowAsync<ScheduleEnvDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("schedule env_vars decryption failed (provider=kms)");
    }

    [Fact]
    public async Task Encrypt_when_kms_throws_surfaces_exception_without_key_in_message()
    {
        var failing = new FailingKmsClient();
        var p = new GoogleKmsScheduleEnvKeyProvider(failing, Opts("projects/secret-project/locations/global/keyRings/kr/cryptoKeys/se/1"));
        var pt = System.Text.Encoding.UTF8.GetBytes("{\"KEY\":\"val\"}");
        var act = () => p.EncryptAsync(pt);
        var ex = (await act.Should().ThrowAsync<ScheduleEnvDecryptException>()).Which;
        ex.Message.Should().NotContain("secret-project");
        ex.Message.Should().Be("schedule env_vars decryption failed (provider=kms)");
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
