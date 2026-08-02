using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Organization = ApiTool.Backend.Data.Entities.Organization;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Unit tests for <see cref="UserExportBundleAssembler"/>.
/// Uses SQLite in-memory to exercise real EF Core queries.
/// </summary>
public sealed class UserExportBundleAssemblerTests : IDisposable
{
    private readonly SqliteConnection _conn;

    private static readonly DateTimeOffset Now = new DateTimeOffset(2026, 5, 18, 10, 0, 0, TimeSpan.Zero);
    private static readonly GdprBundleManifest Manifest = GdprAttributeScanner.Manifest;

    public UserExportBundleAssemblerTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private static IServiceScopeFactory BuildScopeFactory(SqliteConnection conn)
    {
        var services = new ServiceCollection();
        services.AddDbContext<AppDbContext>(opts => opts.UseSqlite(conn));
        return services.BuildServiceProvider().GetRequiredService<IServiceScopeFactory>();
    }

    private async Task<Guid> SeedUserAsync(AppDbContext db, string emailPrefix = "user")
    {
        var id = Guid.NewGuid();
        db.Users.Add(new User { Id = id, Email = $"{emailPrefix}-{id:N}@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();
        return id;
    }

    [Fact]
    public async Task Bundle_contains_exactly_the_seven_InExport_tables()
    {
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);

        var bundle = await UserExportBundleAssembler.AssembleAsync(
            scope.Db, Manifest, userId, Now.UtcDateTime, default);

        var tables = bundle["tables"]!.AsObject();
        tables.Count.Should().Be(7, because: "M18-005 adds deletion_reauth_tokens to InExport");
        var expectedNames = Manifest.InExport.Select(e => e.TableName).ToHashSet();
        tables.Select(kv => kv.Key).Should().BeEquivalentTo(expectedNames);
    }

    [Fact]
    public async Task Empty_user_yields_empty_arrays_per_table()
    {
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);

        var bundle = await UserExportBundleAssembler.AssembleAsync(
            scope.Db, Manifest, userId, Now.UtcDateTime, default);

        var tables = bundle["tables"]!.AsObject();
        // users table should have exactly 1 row (the user itself)
        var users = tables["users"]!.AsArray();
        users.Count.Should().Be(1);
        // all other tables should be empty since we seeded no related data
        foreach (var entry in Manifest.InExport.Where(e => e.TableName != "users"))
        {
            var arr = tables[entry.TableName]!.AsArray();
            arr.Count.Should().Be(0, $"{entry.TableName} should be empty for a freshly seeded user");
        }
    }

    [Fact]
    public async Task Other_users_rows_are_not_included()
    {
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db, "alice");
        var otherUserId = await SeedUserAsync(scope.Db, "bob");

        // Add password reset token for bob only
        scope.Db.PasswordResetTokens.Add(new PasswordResetToken
        {
            Id = Guid.NewGuid(),
            UserId = otherUserId,
            TokenHash = Array.Empty<byte>(),
            IssuedAt = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddHours(1),
        });
        await scope.Db.SaveChangesAsync();

        var bundle = await UserExportBundleAssembler.AssembleAsync(
            scope.Db, Manifest, userId, Now.UtcDateTime, default);

        var tables = bundle["tables"]!.AsObject();
        // alice's bundle should have 0 password_reset_tokens (they belong to bob)
        var resetTokens = tables["password_reset_tokens"]!.AsArray();
        resetTokens.Count.Should().Be(0);
    }

    [Fact]
    public async Task Audit_log_filters_by_actor_id_not_user_id()
    {
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db, "actor");
        var orgId = Guid.NewGuid();
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"torg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ActorId = userId,   // ← filtered via ActorId (not UserId)
            EventType = "test.event",
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        var bundle = await UserExportBundleAssembler.AssembleAsync(
            scope.Db, Manifest, userId, Now.UtcDateTime, default);

        var tables = bundle["tables"]!.AsObject();
        var auditLog = tables["organization_audit_log"]!.AsArray();
        auditLog.Count.Should().Be(1, "the audit log entry with ActorId == userId should be included");
    }

    public void Dispose() => _conn.Dispose();

    [Fact]
    public async Task Bundle_top_level_metadata_is_populated()
    {
        using var scope = OpenScope();
        var userId = await SeedUserAsync(scope.Db);

        var bundle = await UserExportBundleAssembler.AssembleAsync(
            scope.Db, Manifest, userId, Now.UtcDateTime, default);

        bundle["user_id"]!.GetValue<string>().Should().Be(userId.ToString());
        bundle["schema_version"]!.GetValue<int>().Should().Be(1);
        bundle["generated_at"]!.GetValue<string>().Should().NotBeNullOrEmpty();
    }
}
