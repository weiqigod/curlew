// Backfill host tests for team vault encryption (M18-009).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Bootstrap;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.VaultConfig;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>
/// Unit tests for <see cref="TeamVaultBackfillHost"/>.
/// Exercises the critical safety logic: seed-plaintext-rows encryption on first tick,
/// idempotency on already-encrypted rows, and abort on per-row encryption failure.
/// Uses an in-memory SQLite DB via <see cref="TestDb"/> and <see cref="SingletonScopeFactory"/>.
/// Refs: M18-009 review finding #1.
/// </summary>
public sealed class TeamVaultBackfillHostTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;

    public TeamVaultBackfillHostTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.Migrate();
    }

    public ValueTask DisposeAsync() => _scope.DisposeAsync();

    // ── helpers ─────────────────────────────────────────────────────────────

    private TeamVaultBackfillHost BuildHost(ITeamVaultKeyProvider provider)
    {
        var sp = new SingletonScopeFactory(_scope.Db);
        return new TeamVaultBackfillHost(
            new ServiceProviderAdapter(sp),
            provider,
            NullLogger<TeamVaultBackfillHost>.Instance);
    }

    /// <summary>
    /// Wraps a <see cref="SingletonScopeFactory"/> behind <see cref="IServiceProvider"/>
    /// so it can be passed to <see cref="TeamVaultBackfillHost"/>, which calls
    /// <c>sp.CreateScope()</c> via the <see cref="System.IServiceProvider"/> interface.
    /// </summary>
    private sealed class ServiceProviderAdapter(SingletonScopeFactory factory) : IServiceProvider
    {
        public object? GetService(Type serviceType)
        {
            if (serviceType == typeof(Microsoft.Extensions.DependencyInjection.IServiceScopeFactory))
                return factory;
            return null;
        }
    }

    private Guid SeedOrg()
    {
        var orgId = Guid.NewGuid();
        var userId = Guid.NewGuid();
        _scope.Db.Users.Add(new User { Id = userId, Email = $"u-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        _scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"to-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.SaveChanges();
        return orgId;
    }

    // ── tests ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Seeded_plaintext_rows_are_encrypted_on_first_tick()
    {
        // Arrange: seed a team_vaults row that has plaintext but no ciphertext.
        var orgId = SeedOrg();
        _scope.Db.TeamVaults.Add(new TeamVault
        {
            OrgId = orgId,
            TemplateYaml = "team_secrets: {}",
            TemplateJson = "{}",
            TemplateJsonCiphertext = null,   // plaintext only — pre-backfill state
            TemplateJsonKid = null,
            Version = 1,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();

        var provider = new FakeTeamVaultKeyProvider();
        var host = BuildHost(provider);

        // Act
        await host.StartAsync(CancellationToken.None);

        // Assert: row now has ciphertext + kid populated.
        var row = await _scope.Db.TeamVaults.SingleAsync(v => v.OrgId == orgId);
        row.TemplateJsonCiphertext.Should().NotBeNull();
        row.TemplateJsonCiphertext!.Length.Should().BeGreaterThan(0);
        row.TemplateJsonKid.Should().Be(FakeTeamVaultKeyProvider.FakeKid);
    }

    [Fact]
    public async Task Already_encrypted_rows_are_left_alone_on_second_tick()
    {
        // Arrange: seed a row that already has ciphertext (post-backfill state).
        var orgId = SeedOrg();
        var existingCiphertext = new byte[] { 0x01, 0x02, 0x03 };
        _scope.Db.TeamVaults.Add(new TeamVault
        {
            OrgId = orgId,
            TemplateYaml = "team_secrets: {}",
            TemplateJson = "{}",
            TemplateJsonCiphertext = existingCiphertext,
            TemplateJsonKid = FakeTeamVaultKeyProvider.FakeKid,
            Version = 1,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();

        var provider = new FakeTeamVaultKeyProvider();
        var host = BuildHost(provider);

        // Act: run the host a second time (idempotency check).
        await host.StartAsync(CancellationToken.None);

        // Assert: ciphertext is unchanged — the host skipped the already-encrypted row.
        var row = await _scope.Db.TeamVaults.SingleAsync(v => v.OrgId == orgId);
        row.TemplateJsonCiphertext.Should().BeEquivalentTo(existingCiphertext,
            because: "the host must not re-encrypt already-encrypted rows");
        row.TemplateJsonKid.Should().Be(FakeTeamVaultKeyProvider.FakeKid);
    }

    [Fact]
    public async Task Backfill_throws_when_a_row_fails_to_encrypt()
    {
        // Arrange: seed a plaintext row and use a provider that throws on EncryptAsync.
        var orgId = SeedOrg();
        _scope.Db.TeamVaults.Add(new TeamVault
        {
            OrgId = orgId,
            TemplateYaml = "team_secrets: {}",
            TemplateJson = "{}",
            TemplateJsonCiphertext = null,
            TemplateJsonKid = null,
            Version = 1,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();

        var failingProvider = new FailingTeamVaultKeyProvider();
        var host = BuildHost(failingProvider);

        // Act & Assert: StartAsync must propagate the encrypt failure.
        await host.Awaiting(h => h.StartAsync(CancellationToken.None))
            .Should().ThrowAsync<Exception>(
                because: "the host must not silently swallow encrypt failures during backfill");

        // The row should still have no ciphertext — the backfill was not silent.
        var row = await _scope.Db.TeamVaults.SingleAsync(v => v.OrgId == orgId);
        row.TemplateJsonCiphertext.Should().BeNull(
            because: "the failed encrypt should not have persisted partial ciphertext");
    }

    // ── test doubles ─────────────────────────────────────────────────────────

    /// <summary>
    /// A provider that always throws <see cref="InvalidOperationException"/> on
    /// <see cref="EncryptAsync"/> — simulates a broken/unconfigured key provider.
    /// </summary>
    private sealed class FailingTeamVaultKeyProvider : ITeamVaultKeyProvider
    {
        public Task<TeamVaultEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
            => throw new InvalidOperationException("Simulated encryption failure for test.");

        public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
            => Task.FromResult((byte[])ciphertext.Clone());
    }
}
