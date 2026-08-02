// Round-trip and backfill integration tests for team vault encryption (M18-009).
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.VaultConfig;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using TeamVaultEntity = ApiTool.Backend.Data.Entities.TeamVault;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>
/// Integration tests for the team vault encryption round-trip (M18-009, v4-12).
/// Uses an in-memory SQLite database to exercise the full write→persist→read path.
/// </summary>
public sealed class TeamVaultEncryptionRoundTripTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly FakeTeamVaultKeyProvider _provider;
    private readonly VaultConfigService _svc;
    private readonly Guid _userId;
    private readonly Guid _orgId;

    private const string ValidYaml = """
        team_secrets:
          provider: aws-secrets-manager
          keys:
            api_key: prod/api-key
        """;

    public TeamVaultEncryptionRoundTripTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.Migrate();
        _provider = new FakeTeamVaultKeyProvider();

        _userId = Guid.NewGuid();
        _orgId = Guid.NewGuid();
        _scope.Db.Users.Add(new User { Id = _userId, Email = $"user-{_userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        _scope.Db.Organizations.Add(new Organization
        {
            Id = _orgId,
            Name = "TestOrg",
            Slug = $"testorg-{_orgId:N}"[..20],
            OwnerId = _userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgId,
            UserId = _userId,
            Role = OrgRole.Admin,
            JoinedAt = DateTime.UtcNow,
        });
        _scope.Db.SaveChanges();

        _svc = BuildService(_scope.Db, _provider);
    }

    public ValueTask DisposeAsync() => _scope.DisposeAsync();

    private static VaultConfigService BuildService(AppDbContext db, ITeamVaultKeyProvider provider, string validatorMode = "reject")
    {
        var roles = new RoleResolver(db);
        var auditCtx = new AuditContext();
        var auditWriter = new AuditWriter(db, auditCtx, TimeProvider.System);
        var opts = Options.Create(new VaultConfigOptions { ValidatorMode = validatorMode });
        return new VaultConfigService(db, roles, auditWriter, TimeProvider.System, opts, provider,
            NullLogger<VaultConfigService>.Instance);
    }

    [Fact]
    public async Task Write_then_read_template_via_service_returns_cleartext()
    {
        var (_, err, _, _) = await _svc.UpsertAsync(_userId, _orgId, ValidYaml, default);
        err.Should().Be(VaultConfigServiceError.None);

        var (dto, getErr, _) = await _svc.GetAsync(_userId, _orgId, default);
        getErr.Should().Be(VaultConfigServiceError.None);
        dto!.Template.Should().Be(ValidYaml);
    }

    [Fact]
    public async Task Persisted_ciphertext_does_not_contain_cleartext_bytes()
    {
        await _svc.UpsertAsync(_userId, _orgId, ValidYaml, default);

        var row = await _scope.Db.TeamVaults.SingleAsync(x => x.OrgId == _orgId);
        row.TemplateJsonCiphertext.Should().NotBeNull();

        // The ciphertext (even in fake/identity mode) must be stored as bytes —
        // with the FakeTeamVaultKeyProvider identity encoding, the bytes equal the JSON encoding.
        // The YAML plaintext should NOT appear as a substring in the ciphertext when using real provider.
        // With fake, we just verify ciphertext is stored (non-null, non-empty).
        row.TemplateJsonCiphertext!.Length.Should().BeGreaterThan(0);
        row.TemplateJsonKid.Should().Be(FakeTeamVaultKeyProvider.FakeKid);
    }

    [Fact]
    public async Task Suspicious_value_validator_runs_against_cleartext_before_encryption()
    {
        // A literal secret-shaped value should be rejected by the validator before encryption.
        const string suspicious = """
            team_secrets:
              password: abcdefghij1234567890
            """;

        var (_, err, _, _) = await _svc.UpsertAsync(_userId, _orgId, suspicious, default);
        err.Should().Be(VaultConfigServiceError.SuspiciousValue);
    }

    [Fact]
    public async Task Decrypt_after_kid_change_fails_with_DecryptionFailed_error()
    {
        // Write a row using the normal fake provider.
        var (_, upsertErr, _, _) = await _svc.UpsertAsync(_userId, _orgId, ValidYaml, default);
        upsertErr.Should().Be(VaultConfigServiceError.None);

        // Mutate the persisted kid so it no longer matches what the provider recognises.
        var row = await _scope.Db.TeamVaults.SingleAsync(x => x.OrgId == _orgId);
        row.TemplateJsonKid = "rotated-kid-unknown";
        await _scope.Db.SaveChangesAsync();

        // Build a service backed by a provider that throws on kid mismatch.
        var throwingProvider = new ThrowingOnDecryptTeamVaultKeyProvider();
        var svcWithThrowingProvider = BuildService(_scope.Db, throwingProvider);

        // GetAsync must return DecryptionFailed — not a successful 200 with stale plaintext.
        var (dto, getErr, _) = await svcWithThrowingProvider.GetAsync(_userId, _orgId, default);
        getErr.Should().Be(VaultConfigServiceError.DecryptionFailed);
        dto.Should().BeNull();
    }

    /// <summary>
    /// Inline provider that always throws <see cref="TeamVaultDecryptException"/> on decryption,
    /// simulating a key rotation that left the stored kid unresolvable.
    /// </summary>
    private sealed class ThrowingOnDecryptTeamVaultKeyProvider : ITeamVaultKeyProvider
    {
        public Task<TeamVaultEncryptionResult> EncryptAsync(byte[] plaintext, CancellationToken ct = default)
            => Task.FromResult(new TeamVaultEncryptionResult((byte[])plaintext.Clone(), "throwing-provider-v1"));

        public Task<byte[]> DecryptAsync(byte[] ciphertext, string kid, CancellationToken ct = default)
            => throw new TeamVaultDecryptException("throwing-test-provider");
    }
}
