// Refs docs/SPECIFICATION.md:7992 — RFC 6979 deterministic ECDSA required for ES256.
// .NET's ECDsa.SignData uses random nonces (non-deterministic); BouncyCastle provides
// RFC 6979 deterministic nonce generation.
using Org.BouncyCastle.Asn1.X9;
using Org.BouncyCastle.Crypto;
using Org.BouncyCastle.Crypto.Generators;
using Org.BouncyCastle.Crypto.Parameters;
using Org.BouncyCastle.Crypto.Signers;
using Org.BouncyCastle.Math;
using Org.BouncyCastle.Security;
using System.Security.Cryptography;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// RFC 6979 deterministic ES256 signing and key-generation via BouncyCastle.
/// All exported members are thread-safe (stateless).
/// </summary>
internal static class BouncyCastleEs256
{
    private static readonly X9ECParameters Curve = ECNamedCurveTable.GetByName("P-256")!;
    private static readonly ECDomainParameters Domain = new(Curve.Curve, Curve.G, Curve.N, Curve.H, Curve.GetSeed());

    /// <summary>
    /// Generates a new P-256 key pair. Returns the private key as a PKCS#8 PEM string
    /// and the public key as a <see cref="Microsoft.IdentityModel.Tokens.JsonWebKey"/>.
    /// </summary>
    public static (string PrivatePem, JsonWebKey PublicJwk) GenerateKeyPair(string kid)
    {
        var gen = new ECKeyPairGenerator("ECDSA");
        gen.Init(new ECKeyGenerationParameters(Domain, new SecureRandom()));
        var pair = gen.GenerateKeyPair();

        var privatePem = ExportPrivatePem((ECPrivateKeyParameters)pair.Private, (ECPublicKeyParameters)pair.Public);
        var jwk = ExportPublicJwk(kid, (ECPublicKeyParameters)pair.Public);

        return (privatePem, jwk);
    }

    /// <summary>
    /// Signs <paramref name="payload"/> using RFC 6979 deterministic ECDSA with SHA-256.
    /// Returns the raw R||S signature bytes (64 bytes).
    /// </summary>
    public static byte[] SignDeterministic(string privatePem, byte[] payload)
    {
        var (privateKey, _) = ImportPrivatePem(privatePem);

        var signer = new ECDsaSigner(new HMacDsaKCalculator(new Org.BouncyCastle.Crypto.Digests.Sha256Digest()));
        signer.Init(forSigning: true, privateKey);

        // Compute SHA-256 hash of the payload
        var hash = SHA256.HashData(payload);

        var sig = signer.GenerateSignature(hash);
        return EncodeRs(sig[0], sig[1]);
    }

    /// <summary>
    /// Exports the EC public key as a <see cref="JsonWebKey"/> with <c>use=sig</c> and <c>alg=ES256</c>.
    /// </summary>
    private static JsonWebKey ExportPublicJwk(string kid, ECPublicKeyParameters pub)
    {
        var q = pub.Q.Normalize();
        var xBytes = PadTo32(q.AffineXCoord.GetEncoded());
        var yBytes = PadTo32(q.AffineYCoord.GetEncoded());

        return new JsonWebKey
        {
            Kid = kid,
            Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
            Crv = "P-256",
            Use = "sig",
            Alg = "ES256",
            X = Base64UrlEncoder.Encode(xBytes),
            Y = Base64UrlEncoder.Encode(yBytes),
        };
    }

    /// <summary>
    /// Exports the key pair as a PKCS#8 PEM that can be re-imported via <see cref="ImportPrivatePem"/>.
    /// The PEM format is a simple Base64-encoded block for portability.
    /// </summary>
    private static string ExportPrivatePem(ECPrivateKeyParameters priv, ECPublicKeyParameters pub)
    {
        // Use the raw scalar (d) and public point (Q) for a minimal, self-contained PEM.
        // Format: "-----BEGIN EC PRIVATE KEY-----\n<base64>\n-----END EC PRIVATE KEY-----"
        // We use .NET's own ECDsa to serialize to a standard SEC1/PKCS#8 PEM since BouncyCastle's
        // PEM writer requires PemWriter which is straightforward.
        var d = priv.D.ToByteArrayUnsigned();
        var q = pub.Q.Normalize();
        var x = PadTo32(q.AffineXCoord.GetEncoded());
        var y = PadTo32(q.AffineYCoord.GetEncoded());

        // Reconstitute as .NET ECDsa to export as PKCS#8 PEM.
        using var ecdsa = ECDsa.Create(new ECParameters
        {
            Curve = ECCurve.NamedCurves.nistP256,
            D = PadTo32(d),
            Q = new ECPoint { X = x, Y = y },
        });
        return ecdsa.ExportPkcs8PrivateKeyPem();
    }

    /// <summary>Imports a PKCS#8 private key PEM and returns BouncyCastle parameters.</summary>
    private static (ECPrivateKeyParameters priv, ECPublicKeyParameters pub) ImportPrivatePem(string pem)
    {
        using var ecdsa = ECDsa.Create();
        ecdsa.ImportFromPem(pem);
        var p = ecdsa.ExportParameters(includePrivateParameters: true);

        var d = new BigInteger(1, p.D!);
        var q = Domain.Curve.CreatePoint(
            new BigInteger(1, p.Q.X!),
            new BigInteger(1, p.Q.Y!));

        var priv = new ECPrivateKeyParameters("ECDSA", d, Domain);
        var pub = new ECPublicKeyParameters("ECDSA", q, Domain);
        return (priv, pub);
    }

    /// <summary>Encodes R and S as a fixed-width 64-byte R||S concatenation (IEEE P1363).</summary>
    private static byte[] EncodeRs(BigInteger r, BigInteger s)
    {
        var result = new byte[64];
        var rb = r.ToByteArrayUnsigned();
        var sb = s.ToByteArrayUnsigned();
        Buffer.BlockCopy(rb, 0, result, 32 - rb.Length, rb.Length);
        Buffer.BlockCopy(sb, 0, result, 64 - sb.Length, sb.Length);
        return result;
    }

    /// <summary>
    /// Pads a byte array to exactly 32 bytes (left-zero-pads if shorter).
    /// For P-256, coordinate components are always ≤ 32 bytes.
    /// </summary>
    private static byte[] PadTo32(byte[] input)
    {
        if (input.Length == 32) return input;
        if (input.Length > 32) throw new ArgumentException("Coordinate exceeds 32 bytes", nameof(input));
        var padded = new byte[32];
        Buffer.BlockCopy(input, 0, padded, 32 - input.Length, input.Length);
        return padded;
    }
}
