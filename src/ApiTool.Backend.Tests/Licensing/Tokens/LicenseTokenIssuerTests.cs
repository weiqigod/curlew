// Tests for LicenseTokenIssuer — mints License JWTs per spec :7854-7876.
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Tokens;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using ApiTool.Backend.Licensing.Trials;

namespace ApiTool.Backend.Tests.Licensing.Tokens;

public sealed class LicenseTokenIssuerTests : IAsyncDisposable
{
    private readonly string _tmpDir;

    public LicenseTokenIssuerTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_lic_test_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task IssueAsync_emits_license_jwt_with_17_claim_shape_and_typ_license_jwt()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);
        var sut = CreateIssuer(keyProvider);

        var jwt = await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "free",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: Array.Empty<string>(),
            RequestLimit: 1000));

        var (header, payload) = SplitJws(jwt);

        // Header assertions
        header.GetProperty("typ").GetString().Should().Be("license+jwt");
        header.GetProperty("alg").GetString().Should().Be("ES256");
        header.GetProperty("kid").GetString().Should().MatchRegex(@"^[a-z0-9-]{1,64}$");

        // 17 claims per spec :7854-7876
        var requiredClaims = new[]
        {
            "iss", "aud", "sub", "exp", "nbf", "iat", "jti",
            "email", "tier", "features", "request_limit",
            "org_id", "org_role", "device_id",
            "trial_state", "trial_expiry", "grace_until",
        };
        foreach (var claim in requiredClaims)
        {
            payload.TryGetProperty(claim, out _).Should().BeTrue(because: $"claim '{claim}' must be present");
        }

        payload.GetProperty("aud").GetString().Should().Be("apitool-license");
        payload.GetProperty("tier").GetString().Should().Be("free");
        payload.GetProperty("trial_state").GetString().Should().Be("none");

        // trial_expiry must be present with explicit null value (schema-stability contract for M16)
        payload.TryGetProperty("trial_expiry", out var trialExpiry).Should().BeTrue(
            because: "trial_expiry must be present even when null for M16 schema stability");
        trialExpiry.ValueKind.Should().Be(JsonValueKind.Null);

        // grace_until = exp + 14 days
        var exp = payload.GetProperty("exp").GetInt64();
        var grace = payload.GetProperty("grace_until").GetInt64();
        (grace - exp).Should().Be((long)TimeSpan.FromDays(14).TotalSeconds);
    }

    [Fact]
    public async Task IssueAsync_kid_header_points_at_current_signing_key()
    {
        // Behavior #7: header kid MUST match the current key's kid.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);
        var sut = CreateIssuer(keyProvider);

        var activeKid = await keyProvider.GetActiveKidAsync();
        var jwt = await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "free",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: Array.Empty<string>(),
            RequestLimit: 1000));

        var (header, _) = SplitJws(jwt);
        header.GetProperty("kid").GetString().Should().Be(activeKid);
    }

    [Fact]
    public async Task IssueAsync_signature_verifies_with_es256()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, store) = CreateKeyProvider(scope.Db);
        var sut = CreateIssuer(keyProvider);

        var jwt = await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "free",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: Array.Empty<string>(),
            RequestLimit: 1000));

        var parts = jwt.Split('.');
        parts.Should().HaveCount(3, because: "compact JWS must have 3 parts");

        // Verify the signature
        var row = await store.LoadCurrentAsync();
        row.Should().NotBeNull();
        var jwk = JsonSerializer.Deserialize<Microsoft.IdentityModel.Tokens.JsonWebKey>(row!.PublicKeyJwkJson)!;
        var ecdsa = ECDsa.Create();
        ecdsa.ImportParameters(new ECParameters
        {
            Curve = ECCurve.NamedCurves.nistP256,
            Q = new ECPoint
            {
                X = Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(jwk.X),
                Y = Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(jwk.Y),
            },
        });

        var headerPayloadBytes = Encoding.ASCII.GetBytes($"{parts[0]}.{parts[1]}");
        var sigBytes = Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[2]);
        ecdsa.VerifyData(headerPayloadBytes, sigBytes, HashAlgorithmName.SHA256,
            DSASignatureFormat.IeeeP1363FixedFieldConcatenation).Should().BeTrue(
            because: "JWS signature must verify against the active ES256 public key");
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private (FileKeyProvider provider, EfSigningKeyStore store) CreateKeyProvider(
        ApiTool.Backend.Data.AppDbContext db)
    {
        var store = new EfSigningKeyStore(db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "file",
            Env = "test",
            File = new KeyProviderOptions.FileProviderOptions { Dir = _tmpDir },
        });
        var provider = new FileKeyProvider(store, opts, TimeProvider.System,
            NullLogger<FileKeyProvider>.Instance);
        return (provider, store);
    }

    private static LicenseTokenIssuer CreateIssuer(IKeyProvider keyProvider, ITrialStateResolver? resolver = null) =>
        new(keyProvider, Options.Create(new TokenIssuerOptions()), TimeProvider.System,
            resolver ?? new FakeTrialStateResolver());

    private static (JsonElement header, JsonElement payload) SplitJws(string jwt)
    {
        var parts = jwt.Split('.');
        var headerJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[0]));
        var payloadJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[1]));
        return (JsonDocument.Parse(headerJson).RootElement, JsonDocument.Parse(payloadJson).RootElement);
    }
}
