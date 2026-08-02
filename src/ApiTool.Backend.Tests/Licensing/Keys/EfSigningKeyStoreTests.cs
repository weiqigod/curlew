// Tests for EfSigningKeyStore — EF-backed ISigningKeyStore implementation.
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Licensing.Keys;

public sealed class EfSigningKeyStoreTests
{
    [Fact]
    public async Task InsertAsync_then_LoadByKid_round_trips()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = new EfSigningKeyStore(scope.Db);
        var key = MakeKey("dev-es256-202605-aaa111", KeyStatus.Current);

        await store.InsertAsync(key);
        var loaded = await store.LoadByKidAsync("dev-es256-202605-aaa111");

        loaded.Should().NotBeNull();
        loaded!.Kid.Should().Be("dev-es256-202605-aaa111");
        loaded.Algorithm.Should().Be("ES256");
        loaded.Status.Should().Be(KeyStatus.Current);
    }

    [Fact]
    public async Task InsertAsync_two_rows_status_current_throws_DbUpdateException()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = new EfSigningKeyStore(scope.Db);

        await store.InsertAsync(MakeKey("dev-es256-202605-bbb111", KeyStatus.Current));

        // Detach to avoid EF tracking conflict
        scope.Db.ChangeTracker.Clear();

        var act = async () => await store.InsertAsync(MakeKey("dev-es256-202605-bbb222", KeyStatus.Current));
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "idx_signing_keys_current partial unique index prevents two current keys");
    }

    [Fact]
    public async Task LoadCurrentAsync_returns_current_key()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = new EfSigningKeyStore(scope.Db);
        await store.InsertAsync(MakeKey("dev-es256-202605-ccc111", KeyStatus.Current));

        var current = await store.LoadCurrentAsync();

        current.Should().NotBeNull();
        current!.Kid.Should().Be("dev-es256-202605-ccc111");
    }

    [Fact]
    public async Task LoadCurrentAndVerifyingAsync_returns_both_statuses()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = new EfSigningKeyStore(scope.Db);
        await store.InsertAsync(MakeKey("dev-es256-202605-ddd111", KeyStatus.Current));
        await store.InsertAsync(MakeKey("dev-es256-202605-ddd222", KeyStatus.Verifying));
        await store.InsertAsync(MakeKey("dev-es256-202605-ddd333", KeyStatus.Revoked));

        var rows = await store.LoadCurrentAndVerifyingAsync();

        rows.Should().HaveCount(2);
        rows.Select(r => r.Kid).Should().Contain("dev-es256-202605-ddd111");
        rows.Select(r => r.Kid).Should().Contain("dev-es256-202605-ddd222");
        rows.Select(r => r.Kid).Should().NotContain("dev-es256-202605-ddd333");
    }

    [Fact]
    public async Task UpdateStatusAsync_sets_PromotedAt_when_transitioning_to_current()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var store = new EfSigningKeyStore(scope.Db);
        var clock = new FakeClock(new DateTimeOffset(2026, 5, 4, 12, 0, 0, TimeSpan.Zero));

        // Insert a 'next' key and then promote it to 'current'
        await store.InsertAsync(MakeKey("dev-es256-202605-prmt01", KeyStatus.Next));
        var updated = await store.UpdateStatusAsync(
            "dev-es256-202605-prmt01", KeyStatus.Current, revokeReason: null, clock);

        updated.Should().BeTrue();
        scope.Db.ChangeTracker.Clear();
        var row = await store.LoadByKidAsync("dev-es256-202605-prmt01");
        row!.Status.Should().Be(KeyStatus.Current);
        row.PromotedAt.Should().Be(clock.GetUtcNow().UtcDateTime,
            because: "UpdateStatusAsync must set PromotedAt when promoting a key to current");
    }

    private static SigningKey MakeKey(string kid, string status) => new()
    {
        Kid = kid,
        Algorithm = "ES256",
        Status = status,
        PublicKeyJwkJson = "{\"kty\":\"EC\"}",
        CreatedAt = DateTime.UtcNow,
    };
}
