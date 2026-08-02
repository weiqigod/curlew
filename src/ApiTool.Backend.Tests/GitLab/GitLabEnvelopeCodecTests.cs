// Tests for the GitLabEnvelopeCodec wire format (M16-013).
using System.Security.Cryptography;
using ApiTool.Backend.GitLab;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Tests for <see cref="GitLabEnvelopeCodec"/> — verifies the Pack/Unpack/GCM primitives
/// without testing any key-provider logic.
/// </summary>
public sealed class GitLabEnvelopeCodecTests
{
    [Fact]
    public void Round_trip_with_identity_wrap_unwrap_recovers_plaintext()
    {
        var pt = System.Text.Encoding.UTF8.GetBytes("glpat-abcdef0123456789");
        // Identity wrap: store the raw DEK bytes as the "wrapped DEK" — focuses on codec, not wrapping.
        var (dek, nonce, ct, tag) = GitLabEnvelopeCodec.GenerateAndEncrypt(pt);
        var blob = GitLabEnvelopeCodec.Pack(dek, nonce, ct, tag);
        var (wrappedDek2, nonce2, ct2, tag2) = GitLabEnvelopeCodec.Unpack(blob);
        var dec = GitLabEnvelopeCodec.DecryptWithDek(wrappedDek2, nonce2, ct2, tag2);
        dec.Should().Equal(pt);
    }

    [Fact]
    public void Decrypt_with_wrong_version_throws()
    {
        var pt = new byte[] { 9, 9, 9 };
        var (dek, nonce, ct, tag) = GitLabEnvelopeCodec.GenerateAndEncrypt(pt);
        var blob = GitLabEnvelopeCodec.Pack(dek, nonce, ct, tag);
        blob[0] = 0xFF;
        Action act = () => GitLabEnvelopeCodec.Unpack(blob);
        act.Should().Throw<CryptographicException>().WithMessage("*version*");
    }

    [Fact]
    public void Decrypt_truncated_blob_throws()
    {
        Action act = () => GitLabEnvelopeCodec.Unpack(new byte[] { 0x01, 0, 0 });
        act.Should().Throw<CryptographicException>().WithMessage("*trunc*");
    }

    [Fact]
    public void GenerateAndEncrypt_produces_random_nonce_per_call()
    {
        var pt = new byte[] { 1, 2, 3 };
        var (_, nonce1, _, _) = GitLabEnvelopeCodec.GenerateAndEncrypt(pt);
        var (_, nonce2, _, _) = GitLabEnvelopeCodec.GenerateAndEncrypt(pt);
        nonce1.Should().NotEqual(nonce2);
    }
}
