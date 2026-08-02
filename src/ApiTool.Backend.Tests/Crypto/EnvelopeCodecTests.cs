// Tests for the shared EnvelopeCodec wire format (M18-009).
using System.Security.Cryptography;
using ApiTool.Backend.Crypto;

namespace ApiTool.Backend.Tests.Crypto;

/// <summary>
/// Tests for <see cref="EnvelopeCodec"/> — verifies the Pack/Unpack/GCM primitives
/// without testing any key-provider logic.
/// </summary>
public sealed class EnvelopeCodecTests
{
    [Fact]
    public void Round_trip_with_identity_wrap_unwrap_recovers_plaintext()
    {
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-abcdef0123456789");
        // Identity wrap: store the raw DEK bytes as the "wrapped DEK" — focuses on codec, not wrapping.
        var (dek, nonce, ct, tag) = EnvelopeCodec.GenerateAndEncrypt(pt);
        var blob = EnvelopeCodec.Pack(dek, nonce, ct, tag);
        var (wrappedDek2, nonce2, ct2, tag2) = EnvelopeCodec.Unpack(blob);
        var dec = EnvelopeCodec.DecryptWithDek(wrappedDek2, nonce2, ct2, tag2);
        dec.Should().Equal(pt);
    }

    [Fact]
    public void Decrypt_with_wrong_version_throws()
    {
        var pt = new byte[] { 9, 9, 9 };
        var (dek, nonce, ct, tag) = EnvelopeCodec.GenerateAndEncrypt(pt);
        var blob = EnvelopeCodec.Pack(dek, nonce, ct, tag);
        blob[0] = 0xFF;
        Action act = () => EnvelopeCodec.Unpack(blob);
        act.Should().Throw<CryptographicException>().WithMessage("*version*");
    }

    [Fact]
    public void Decrypt_truncated_blob_throws()
    {
        Action act = () => EnvelopeCodec.Unpack(new byte[] { 0x01, 0, 0 });
        act.Should().Throw<CryptographicException>().WithMessage("*trunc*");
    }

    [Fact]
    public void GenerateAndEncrypt_produces_random_nonce_per_call()
    {
        var pt = new byte[] { 1, 2, 3 };
        var (_, nonce1, _, _) = EnvelopeCodec.GenerateAndEncrypt(pt);
        var (_, nonce2, _, _) = EnvelopeCodec.GenerateAndEncrypt(pt);
        nonce1.Should().NotEqual(nonce2);
    }
}
