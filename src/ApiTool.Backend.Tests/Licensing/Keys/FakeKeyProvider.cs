// In-memory IKeyProvider for integration tests — no file I/O, no DB dependency.
using System.Security.Cryptography;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Licensing.Keys;

/// <summary>
/// Deterministic <see cref="IKeyProvider"/> for integration tests.
/// Uses a locally-generated EC key pair in memory — no file I/O, no database rows.
/// Registered by <see cref="TestInfrastructure.BackendFactory"/> to avoid the
/// file-key/shared-DB contamination problem that arises when <see cref="InternalKeysEndpointsTests"/>
/// rotates the signing key in the shared in-memory database.
/// <para>
/// Call <see cref="SimulateKeyRotation"/> to replace the active key with a freshly-generated
/// pair and update the kid, enabling integration tests that verify Behavior #7 (kid in JWS
/// header after key rotation mid-flight).
/// </para>
/// </summary>
public sealed class FakeKeyProvider : IKeyProvider
{
    private const string DefaultKid = "test-es256-fakeprovider";

    private ECDsa _key = ECDsa.Create(ECCurve.NamedCurves.nistP256);
    private string _kid = DefaultKid;
    private JsonWebKeySet _jwks = BuildJwks(DefaultKid, ECDsa.Create(ECCurve.NamedCurves.nistP256));

    public FakeKeyProvider()
    {
        _jwks = BuildJwks(_kid, _key);
    }

    /// <summary>
    /// Replaces the active key with a newly-generated EC pair and a new kid.
    /// Returns the new kid so callers can assert it appears in subsequent JWT headers.
    /// </summary>
    public string SimulateKeyRotation()
    {
        var newKey = ECDsa.Create(ECCurve.NamedCurves.nistP256);
        var newKid = $"rotated-{Guid.NewGuid():N}";
        _key  = newKey;
        _kid  = newKid;
        _jwks = BuildJwks(newKid, newKey);
        return newKid;
    }

    /// <inheritdoc/>
    public Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default)
    {
        var hash = SHA256.HashData(payload);
        var sig  = _key.SignHash(hash, DSASignatureFormat.IeeeP1363FixedFieldConcatenation);
        return Task.FromResult(new SignatureResult(_kid, sig));
    }

    /// <inheritdoc/>
    public Task<string> GetActiveKidAsync(CancellationToken ct = default)
        => Task.FromResult(_kid);

    /// <inheritdoc/>
    public Task<JsonWebKeySet> GetVerificationJwksAsync(CancellationToken ct = default)
        => Task.FromResult(_jwks);

    private static JsonWebKeySet BuildJwks(string kid, ECDsa key)
    {
        var p   = key.ExportParameters(includePrivateParameters: false);
        var jwk = new JsonWebKey
        {
            Kid = kid,
            Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
            Crv = "P-256",
            Use = "sig",
            Alg = "ES256",
            X   = Base64UrlEncoder.Encode(p.Q.X!),
            Y   = Base64UrlEncoder.Encode(p.Q.Y!),
        };
        var jwks = new JsonWebKeySet();
        jwks.Keys.Add(jwk);
        return jwks;
    }
}
