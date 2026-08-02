// Tests for FileKeyProvider — file-based IKeyProvider implementation.
// Refs docs/SPECIFICATION.md:8040, 8060.
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Licensing.Keys;

public sealed class FileKeyProviderTests : IAsyncDisposable
{
    private readonly string _tmpDir;
    private readonly FakeClock _clock = new(new DateTimeOffset(2026, 5, 4, 0, 0, 0, TimeSpan.Zero));

    public FileKeyProviderTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_test_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task BootstrapAsync_with_empty_db_creates_current_key_with_pem_file_and_db_row()
    {
        // Behavior #1
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);

        var kid = await provider.GetActiveKidAsync();

        // DB row created with status='current'
        var row = await store.LoadCurrentAsync();
        row.Should().NotBeNull();
        row!.Kid.Should().Be(kid);
        row.Status.Should().Be(KeyStatus.Current);
        row.Algorithm.Should().Be("ES256");
        row.PublicKeyJwkJson.Should().Contain("\"kty\":\"EC\"");

        // PEM file created on disk
        var pemPath = Path.Combine(_tmpDir, $"{kid}.pem");
        File.Exists(pemPath).Should().BeTrue(because: "private key PEM must be written to disk");

        // kid format matches allowlist
        kid.Should().MatchRegex(@"^[a-z0-9-]{1,64}$");
    }

    [Fact]
    public async Task SignAsync_signature_verifies_against_active_public_jwk_with_es256()
    {
        // Behavior #2
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);

        var payload = Encoding.UTF8.GetBytes("hello world");
        var result = await provider.SignAsync(payload);

        // Load public JWK from store
        var row = await store.LoadByKidAsync(result.Kid);
        row.Should().NotBeNull();

        // Deserialise JWK and verify
        var jwk = JsonSerializer.Deserialize<JsonWebKey>(row!.PublicKeyJwkJson)!;
        var ecdsa = ECDsa.Create();
        ecdsa.ImportParameters(new ECParameters
        {
            Curve = ECCurve.NamedCurves.nistP256,
            Q = new ECPoint
            {
                X = Base64UrlEncoder.DecodeBytes(jwk.X),
                Y = Base64UrlEncoder.DecodeBytes(jwk.Y),
            },
        });

        // result.Signature is raw R||S (64 bytes)
        var valid = ecdsa.VerifyData(payload, result.Signature, HashAlgorithmName.SHA256, DSASignatureFormat.IeeeP1363FixedFieldConcatenation);
        valid.Should().BeTrue(because: "ES256 signature must verify against the stored public key");
    }

    [Fact]
    public async Task SignAsync_is_deterministic_for_same_payload_RFC_6979()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (_, provider) = CreateProviderAndStore(scope.Db);

        var payload = Encoding.UTF8.GetBytes("hello");
        var sig1 = await provider.SignAsync(payload);
        var sig2 = await provider.SignAsync(payload);

        sig1.Signature.Should().Equal(sig2.Signature,
            because: "FileKeyProvider must use RFC 6979 deterministic ECDSA");
    }

    [Theory]
    [InlineData("../keys/secret")]
    [InlineData("dev/../etc/passwd")]
    [InlineData("KID-WITH-CAPS")]
    [InlineData("")]
    public async Task GetKeyForSigningAsync_invalid_kid_throws_InvalidKidFormat_before_db_lookup(string badKid)
    {
        // Behavior #4
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (_, provider) = CreateProviderAndStore(scope.Db);

        var act = async () => await provider.GetKeyForSigningAsync(badKid, CancellationToken.None);
        await act.Should().ThrowAsync<InvalidKidFormatException>(
            because: "kid validation must reject invalid format before any DB lookup");
    }

    [Fact]
    public async Task GetVerificationJwksAsync_returns_current_and_verifying_keys()
    {
        // Behavior #6
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);

        // Bootstrap a 'current' key
        await provider.GetActiveKidAsync();

        // Manually insert a 'verifying' key
        scope.Db.ChangeTracker.Clear();
        var verifyingKid = "dev-es256-202604-aabbcc";
        await store.InsertAsync(new SigningKey
        {
            Kid = verifyingKid,
            Algorithm = "ES256",
            Status = KeyStatus.Verifying,
            PublicKeyJwkJson = "{\"kty\":\"EC\",\"crv\":\"P-256\",\"use\":\"sig\",\"alg\":\"ES256\",\"kid\":\"" + verifyingKid + "\",\"x\":\"aaaa\",\"y\":\"bbbb\"}",
            CreatedAt = DateTime.UtcNow,
        });

        var jwks = await provider.GetVerificationJwksAsync();

        jwks.Keys.Should().HaveCountGreaterOrEqualTo(2,
            because: "JWKS must include both the current and verifying keys");
    }

    [Fact, Trait("Platform", "Unix")]
    public async Task BootstrapAsync_writes_pem_file_with_mode_0600_on_unix()
    {
        if (!OperatingSystem.IsLinux() && !OperatingSystem.IsMacOS()) return;

        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (_, provider) = CreateProviderAndStore(scope.Db);

        var kid = await provider.GetActiveKidAsync();
        var pemPath = Path.Combine(_tmpDir, $"{kid}.pem");

        var mode = File.GetUnixFileMode(pemPath);
        mode.Should().Be(UnixFileMode.UserRead | UnixFileMode.UserWrite,
            because: "private key PEM must have 0600 permissions to prevent unauthorised access");
    }

    private (ISigningKeyStore store, FileKeyProvider provider) CreateProviderAndStore(
        ApiTool.Backend.Data.AppDbContext db)
    {
        var store = new EfSigningKeyStore(db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "file",
            Env = "dev",
            File = new KeyProviderOptions.FileProviderOptions { Dir = _tmpDir },
        });
        var provider = new FileKeyProvider(store, opts, _clock,
            NullLogger<FileKeyProvider>.Instance);
        return (store, provider);
    }
}
