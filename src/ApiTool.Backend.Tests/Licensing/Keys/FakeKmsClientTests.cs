using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Tests.Licensing.Keys;

namespace ApiTool.Backend.Tests.Licensing.Keys;

/// <summary>
/// Tests for FakeKmsClient — verifies that the new RSA signing method records calls
/// and returns a verifiable signature.
/// </summary>
public sealed class FakeKmsClientTests
{
    [Fact]
    public async Task AsymmetricSignRsaPkcs1Sha256Async_records_call_and_returns_verifiable_signature()
    {
        using var fake = new FakeKmsClient();
        var payload = Encoding.UTF8.GetBytes("hello");
        var sig = await fake.AsymmetricSignRsaPkcs1Sha256Async("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1", payload);
        fake.RsaSignCalls.Should().HaveCount(1);
        sig.Length.Should().Be(256);  // 2048-bit RSA signature = 256 bytes
        var verified = fake.GetLocalRsaPublicKey().VerifyData(
            payload, sig, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        verified.Should().BeTrue();
    }
}
