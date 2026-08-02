// Tests for the symmetric EncryptAsync / DecryptAsync methods added to IKmsClient (M16-013).
// These methods are used by GoogleKmsGitLabKeyProvider to wrap per-row DEKs.
using System.Security.Cryptography;
using ApiTool.Backend.Tests.Licensing.Keys;

namespace ApiTool.Backend.Tests.Licensing.Keys;

/// <summary>
/// Tests verifying the symmetric encrypt/decrypt extensions on <see cref="FakeKmsClient"/>.
/// </summary>
public sealed class FakeKmsClientEncryptDecryptTests
{
    [Fact]
    public async Task Encrypt_then_Decrypt_round_trips_plaintext()
    {
        using var kms = new FakeKmsClient();
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-xxxxxxxxxxxxxxxxxxxx");
        var ct = await kms.EncryptAsync("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", pt);
        var dec = await kms.DecryptAsync("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", ct);
        dec.Should().Equal(pt);
    }

    [Fact]
    public async Task Encrypt_produces_different_ciphertext_each_call_for_same_plaintext()
    {
        using var kms = new FakeKmsClient();
        var pt = System.Text.Encoding.UTF8.GetBytes("same input");
        var ct1 = await kms.EncryptAsync("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", pt);
        var ct2 = await kms.EncryptAsync("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", pt);
        ct1.Should().NotEqual(ct2);  // nonce randomization
    }

    [Fact]
    public async Task Decrypt_with_wrong_key_id_throws()
    {
        using var kms = new FakeKmsClient();
        var ct = await kms.EncryptAsync("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", new byte[] { 1, 2, 3 });
        var act = () => kms.DecryptAsync("projects/q/locations/global/keyRings/kr/cryptoKeys/different/cryptoKeyVersions/2", ct);
        await act.Should().ThrowAsync<Exception>();
    }
}
