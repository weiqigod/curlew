// Tests for SigningKeyRotator — ISigningKeyRotator implementation.
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Licensing.Keys;

public sealed class SigningKeyRotatorTests : IAsyncDisposable
{
    private readonly string _tmpDir;
    private readonly FakeClock _clock = new(new DateTimeOffset(2026, 5, 4, 0, 0, 0, TimeSpan.Zero));

    public SigningKeyRotatorTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_test_rot_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir)) Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task RotateAsync_promotes_next_to_current_and_demotes_old_current_to_verifying()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);
        var rotator = new SigningKeyRotator(store, provider, _clock);

        // Bootstrap a current key
        var oldKid = await provider.GetActiveKidAsync();
        scope.Db.ChangeTracker.Clear();

        // Manually insert a 'next' key
        var nextKid = "dev-es256-202605-next01";
        await store.InsertAsync(MakeKey(nextKid, KeyStatus.Next));

        var newKid = await rotator.RotateAsync(emergency: false, reason: null);

        newKid.Should().Be(nextKid, because: "next key should be promoted to current");
        scope.Db.ChangeTracker.Clear();

        var newCurrent = await store.LoadCurrentAsync();
        newCurrent!.Kid.Should().Be(nextKid);

        var oldRow = await store.LoadByKidAsync(oldKid);
        oldRow!.Status.Should().Be(KeyStatus.Verifying,
            because: "normal rotation demotes old current to verifying");
    }

    [Fact]
    public async Task RotateAsync_emergency_marks_old_current_as_revoked_with_reason()
    {
        // Behavior #7
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);
        var rotator = new SigningKeyRotator(store, provider, _clock);

        var oldKid = await provider.GetActiveKidAsync();
        scope.Db.ChangeTracker.Clear();

        var nextKid = "dev-es256-202605-emrg01";
        await store.InsertAsync(MakeKey(nextKid, KeyStatus.Next));

        var newKid = await rotator.RotateAsync(emergency: true, reason: "compromise");

        newKid.Should().Be(nextKid);
        scope.Db.ChangeTracker.Clear();

        var oldRow = await store.LoadByKidAsync(oldKid);
        oldRow!.Status.Should().Be(KeyStatus.Revoked,
            because: "emergency rotation revokes the old current key");
        oldRow.RevokeReason.Should().Be("compromise");
        oldRow.RevokedAt.Should().NotBeNull();
    }

    [Fact]
    public async Task RotateAsync_jwks_reflects_new_kid_after_rotation()
    {
        // Behavior #7 cont'd
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);
        var rotator = new SigningKeyRotator(store, provider, _clock);

        var oldKid = await provider.GetActiveKidAsync();
        scope.Db.ChangeTracker.Clear();

        var nextKid = "dev-es256-202605-jkws01";
        await store.InsertAsync(MakeKey(nextKid, KeyStatus.Next));

        await rotator.RotateAsync(emergency: true, reason: "test");
        scope.Db.ChangeTracker.Clear();

        var jwks = await provider.GetVerificationJwksAsync();
        // The old (revoked) key must not appear; the new current must appear.
        jwks.Keys.Select(k => k.Kid).Should().Contain(nextKid);
        // Revoked keys are excluded from JWKS
        jwks.Keys.Select(k => k.Kid).Should().NotContain(oldKid);
    }

    [Fact]
    public async Task RotateAsync_when_no_next_key_provisions_new_current_directly()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);
        var rotator = new SigningKeyRotator(store, provider, _clock);

        // No 'current' or 'next' key exists — first rotation bootstraps
        var newKid = await rotator.RotateAsync(emergency: false, reason: null);

        newKid.Should().MatchRegex(@"^[a-z0-9-]{1,64}$");
        var current = await store.LoadCurrentAsync();
        current!.Kid.Should().Be(newKid);
    }

    [Fact]
    public async Task RotateAsync_emergency_with_no_next_key_revokes_current_and_bootstraps_fresh()
    {
        // Review finding #1: emergency rotation with no staged 'next' key must NOT silently return
        // the same kid. It must revoke the compromised current key and bootstrap a fresh one.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (store, provider) = CreateProviderAndStore(scope.Db);
        var rotator = new SigningKeyRotator(store, provider, _clock);

        // Bootstrap a current key (no 'next' staged)
        var oldKid = await provider.GetActiveKidAsync();
        scope.Db.ChangeTracker.Clear();

        var newKid = await rotator.RotateAsync(emergency: true, reason: "compromise");

        // The new kid must be DIFFERENT from the old compromised key
        newKid.Should().NotBe(oldKid,
            because: "emergency rotation must produce a new key, not return the compromised one");
        newKid.Should().MatchRegex(@"^[a-z0-9-]{1,64}$");

        scope.Db.ChangeTracker.Clear();

        // The old key must be revoked
        var oldRow = await store.LoadByKidAsync(oldKid);
        oldRow!.Status.Should().Be(KeyStatus.Revoked,
            because: "emergency rotation must revoke the current key even when no next key is staged");
        oldRow.RevokeReason.Should().Be("compromise");
        oldRow.RevokedAt.Should().NotBeNull();

        // A fresh current key must exist
        var newCurrent = await store.LoadCurrentAsync();
        newCurrent.Should().NotBeNull();
        newCurrent!.Kid.Should().Be(newKid);
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

    private static SigningKey MakeKey(string kid, string status) => new()
    {
        Kid = kid,
        Algorithm = "ES256",
        Status = status,
        PublicKeyJwkJson = "{\"kty\":\"EC\",\"crv\":\"P-256\",\"use\":\"sig\",\"alg\":\"ES256\",\"kid\":\"" + kid + "\",\"x\":\"aaaa\",\"y\":\"bbbb\"}",
        CreatedAt = DateTime.UtcNow,
    };
}
