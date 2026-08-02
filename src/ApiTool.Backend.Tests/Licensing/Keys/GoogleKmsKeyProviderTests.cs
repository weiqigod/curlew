// Tests for GoogleKmsKeyProvider — KMS-backed IKeyProvider implementation.
// Uses FakeKmsClient to avoid live GCP calls (hard rule: no real-world cost).
using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Licensing.Keys;

public sealed class GoogleKmsKeyProviderTests : IAsyncDisposable
{
    private readonly string _tmpDir;
    private readonly FakeClock _clock = new(new DateTimeOffset(2026, 5, 4, 0, 0, 0, TimeSpan.Zero));

    public GoogleKmsKeyProviderTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_kms_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir)) Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task SignAsync_dispatches_to_kms_asymmetric_sign_and_records_kms_key_id()
    {
        // Behavior #3 — no live GCP calls; FakeKmsClient returns deterministic sigs.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var fakeKms = new FakeKmsClient();
        var store = new EfSigningKeyStore(scope.Db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "kms",
            Env = "prod",
            Kms = new KeyProviderOptions.KmsProviderOptions
            {
                ProjectId = "my-project",
                LocationId = "global",
                KeyRing = "apitool-kr",
            },
        });
        var provider = new GoogleKmsKeyProvider(fakeKms, store, opts, _clock,
            NullLogger<GoogleKmsKeyProvider>.Instance);

        var result = await provider.SignAsync(Encoding.UTF8.GetBytes("payload"));

        fakeKms.AsymmetricSignCalls.Should().HaveCount(1,
            because: "signing must dispatch exactly one call to the KMS AsymmetricSign API");
        fakeKms.AsymmetricSignCalls[0].KmsKeyId.Should()
            .StartWith("projects/my-project/locations/global/keyRings/apitool-kr/");

        var stored = await store.LoadByKidAsync(result.Kid);
        stored.Should().NotBeNull();
        stored!.KmsKeyId.Should().NotBeNullOrEmpty(
            because: "the signing_keys row must record the kms_key_id");
        stored.PublicKeyJwkJson.Should().Contain("\"kty\":\"EC\"",
            because: "the public key JWK must be cached in the signing_keys row");
    }

    [Fact]
    public async Task SignAsync_on_second_call_reuses_existing_current_key()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var fakeKms = new FakeKmsClient();
        var store = new EfSigningKeyStore(scope.Db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "kms",
            Env = "prod",
            Kms = new KeyProviderOptions.KmsProviderOptions
            {
                ProjectId = "p",
                LocationId = "global",
                KeyRing = "kr",
            },
        });
        var provider = new GoogleKmsKeyProvider(fakeKms, store, opts, _clock,
            NullLogger<GoogleKmsKeyProvider>.Instance);

        var first = await provider.SignAsync(Encoding.UTF8.GetBytes("first"));
        scope.Db.ChangeTracker.Clear();
        var second = await provider.SignAsync(Encoding.UTF8.GetBytes("second"));

        first.Kid.Should().Be(second.Kid, because: "the same current key should be reused");
        // Only one KMS key-creation call, but two sign calls
        fakeKms.AsymmetricSignCalls.Should().HaveCount(2);
    }

    [Fact]
    public async Task SignAsync_when_current_key_has_null_KmsKeyId_throws_InvalidOperationException()
    {
        // Review finding #2: if a current key was created by FileKeyProvider (KmsKeyId=null)
        // and the server is restarted with Mode=kms, SignAsync must throw a clear error,
        // not a NullReferenceException.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var fakeKms = new FakeKmsClient();
        var store = new EfSigningKeyStore(scope.Db);

        // Simulate a file-backed current key row (KmsKeyId is null)
        var fileBacked = new ApiTool.Backend.Data.Entities.SigningKey
        {
            Kid = "dev-es256-202605-filebk",
            Algorithm = "ES256",
            Status = KeyStatus.Current,
            KmsKeyId = null,  // file-backed — no KMS key id
            PublicKeyJwkJson = "{\"kty\":\"EC\",\"crv\":\"P-256\",\"use\":\"sig\",\"alg\":\"ES256\",\"kid\":\"dev-es256-202605-filebk\",\"x\":\"aaaa\",\"y\":\"bbbb\"}",
            CreatedAt = DateTime.UtcNow,
            PromotedAt = DateTime.UtcNow,
        };
        await store.InsertAsync(fileBacked);

        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "kms",
            Env = "prod",
            Kms = new KeyProviderOptions.KmsProviderOptions
            {
                ProjectId = "p",
                LocationId = "global",
                KeyRing = "kr",
            },
        });
        var provider = new GoogleKmsKeyProvider(fakeKms, store, opts, _clock,
            NullLogger<GoogleKmsKeyProvider>.Instance);

        // Must throw a meaningful InvalidOperationException, not a NullReferenceException
        var act = async () => await provider.SignAsync(System.Text.Encoding.UTF8.GetBytes("payload"));
        await act.Should().ThrowAsync<InvalidOperationException>(
            because: "a file-backed key row with null KmsKeyId must produce a clear error, not NRE")
            .WithMessage("*kms_key_id*");
    }

    [Fact]
    public async Task GetVerificationJwksAsync_returns_cached_public_key_jwk()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var fakeKms = new FakeKmsClient();
        var store = new EfSigningKeyStore(scope.Db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "kms",
            Env = "prod",
            Kms = new KeyProviderOptions.KmsProviderOptions
            {
                ProjectId = "p",
                LocationId = "global",
                KeyRing = "kr",
            },
        });
        var provider = new GoogleKmsKeyProvider(fakeKms, store, opts, _clock,
            NullLogger<GoogleKmsKeyProvider>.Instance);

        // Bootstrap by signing
        var result = await provider.SignAsync(Encoding.UTF8.GetBytes("bootstrap"));

        var jwks = await provider.GetVerificationJwksAsync();
        jwks.Keys.Should().HaveCountGreaterOrEqualTo(1);
        jwks.Keys.Any(k => k.Kid == result.Kid).Should().BeTrue();
    }
}
