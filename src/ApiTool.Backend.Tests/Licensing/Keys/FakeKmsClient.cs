// Deterministic fake IKmsClient for tests — no GCP calls.
using System.Security.Cryptography;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Licensing.Keys;

/// <summary>
/// Records calls to AsymmetricSign and GetPublicKey so tests can assert on call shape
/// without ever hitting Google Cloud KMS.
/// Uses a locally-generated EC key for deterministic ES256 signatures and a locally-generated
/// RSA key for deterministic RS256 signatures (GitHub App JWT path).
/// Also implements symmetric EncryptAsync/DecryptAsync for GitLab PAT DEK-wrapping tests.
/// Each unique kmsKeyId gets its own 32-byte AES-256 local key; decryption with a different
/// key id (or unknown key id) throws <see cref="CryptographicException"/>.
/// </summary>
public sealed class FakeKmsClient : IKmsClient, IDisposable
{
    private readonly ECDsa _localKey = ECDsa.Create(ECCurve.NamedCurves.nistP256);
    private readonly RSA _localRsaKey = RSA.Create(2048);

    // Per-key-id symmetric keys for fake EncryptAsync / DecryptAsync.
    private readonly Dictionary<string, byte[]> _symmetricKeys = new(StringComparer.Ordinal);

    /// <summary>All calls made to <see cref="AsymmetricSignAsync"/>.</summary>
    public List<AsymmetricSignCall> AsymmetricSignCalls { get; } = new();

    /// <summary>All calls made to <see cref="AsymmetricSignRsaPkcs1Sha256Async"/>.</summary>
    public List<RsaSignCall> RsaSignCalls { get; } = new();

    /// <inheritdoc/>
    public Task<byte[]> AsymmetricSignAsync(string kmsKeyId, byte[] payload, CancellationToken ct = default)
    {
        AsymmetricSignCalls.Add(new AsymmetricSignCall(kmsKeyId, payload));
        // Return a deterministic-ish signature (random nonce is fine for fake — tests don't verify the sig bytes)
        var hash = SHA256.HashData(payload);
        var sig = _localKey.SignHash(hash, DSASignatureFormat.IeeeP1363FixedFieldConcatenation);
        return Task.FromResult(sig);
    }

    /// <inheritdoc/>
    public Task<byte[]> AsymmetricSignRsaPkcs1Sha256Async(string kmsKeyId, byte[] payload, CancellationToken ct = default)
    {
        RsaSignCalls.Add(new RsaSignCall(kmsKeyId, payload));
        var sig = _localRsaKey.SignData(payload, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        return Task.FromResult(sig);
    }

    /// <summary>Returns the local RSA public key for signature verification in tests.</summary>
    public RSA GetLocalRsaPublicKey() => _localRsaKey;

    /// <inheritdoc/>
    public Task<string> GetPublicKeyJwkAsync(string kmsKeyId, string kid, CancellationToken ct = default)
    {
        var p = _localKey.ExportParameters(includePrivateParameters: false);
        var jwk = new JsonWebKey
        {
            Kid = kid,
            Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
            Crv = "P-256",
            Use = "sig",
            Alg = "ES256",
            X = Base64UrlEncoder.Encode(p.Q.X!),
            Y = Base64UrlEncoder.Encode(p.Q.Y!),
        };
        return Task.FromResult(JsonSerializer.Serialize(jwk));
    }

    /// <inheritdoc/>
    public Task<byte[]> EncryptAsync(string kmsKeyId, byte[] plaintext, CancellationToken ct = default)
    {
        var key = GetOrCreateSymmetricKey(kmsKeyId);
        var nonce = RandomNumberGenerator.GetBytes(12);
        var ct2 = new byte[plaintext.Length];
        var tag = new byte[16];
        using (var aes = new AesGcm(key, tagSizeInBytes: 16))
            aes.Encrypt(nonce, plaintext, ct2, tag);
        // Wire format: [12 nonce][N ciphertext][16 tag]
        var result = new byte[12 + ct2.Length + 16];
        Buffer.BlockCopy(nonce, 0, result, 0, 12);
        Buffer.BlockCopy(ct2, 0, result, 12, ct2.Length);
        Buffer.BlockCopy(tag, 0, result, 12 + ct2.Length, 16);
        return Task.FromResult(result);
    }

    /// <inheritdoc/>
    public Task<byte[]> DecryptAsync(string kmsKeyId, byte[] ciphertext, CancellationToken ct = default)
    {
        if (!_symmetricKeys.TryGetValue(kmsKeyId, out var key))
            throw new CryptographicException($"FakeKmsClient: unknown kmsKeyId '{kmsKeyId}'");
        if (ciphertext.Length < 12 + 16)
            throw new CryptographicException("FakeKmsClient: ciphertext too short");
        var nonce = new byte[12];
        var tag = new byte[16];
        var ctLen = ciphertext.Length - 12 - 16;
        var ct2 = new byte[ctLen];
        Buffer.BlockCopy(ciphertext, 0, nonce, 0, 12);
        Buffer.BlockCopy(ciphertext, 12, ct2, 0, ctLen);
        Buffer.BlockCopy(ciphertext, 12 + ctLen, tag, 0, 16);
        var plaintext = new byte[ctLen];
        using (var aes = new AesGcm(key, tagSizeInBytes: 16))
            aes.Decrypt(nonce, ct2, tag, plaintext);
        return Task.FromResult(plaintext);
    }

    private byte[] GetOrCreateSymmetricKey(string kmsKeyId)
    {
        if (!_symmetricKeys.TryGetValue(kmsKeyId, out var key))
        {
            key = RandomNumberGenerator.GetBytes(32);
            _symmetricKeys[kmsKeyId] = key;
        }
        return key;
    }

    /// <summary>Disposes the locally-generated EC and RSA keys.</summary>
    public void Dispose()
    {
        _localKey.Dispose();
        _localRsaKey.Dispose();
    }

    /// <summary>A recorded AsymmetricSign (EC) invocation.</summary>
    public sealed record AsymmetricSignCall(string KmsKeyId, byte[] Payload);

    /// <summary>A recorded AsymmetricSignRsaPkcs1Sha256 (RSA) invocation.</summary>
    public sealed record RsaSignCall(string KmsKeyId, byte[] Payload);
}
