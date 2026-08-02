using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>
/// Unit tests for <see cref="LastAdminProtectionService"/> — classifies owned orgs
/// into "blocking" (sole owner + other members present) and "cascade" (sole owner,
/// zero other members), per spec v4-7.
/// Uses SQLite in-memory for real LINQ behaviour.
/// </summary>
public sealed class LastAdminProtectionServiceTests : IAsyncDisposable
{
    private readonly SqliteConnection _conn;

    public LastAdminProtectionServiceTests()
    {
        _conn = new SqliteConnection("DataSource=:memory:");
        _conn.Open();
        using var scope = TestDb.CreateOpenFromConnection(_conn);
        scope.Db.Database.EnsureCreated();
    }

    public async ValueTask DisposeAsync()
    {
        await _conn.DisposeAsync();
    }

    private TestDbScope OpenScope() => TestDb.CreateOpenFromConnection(_conn);

    private LastAdminProtectionService BuildService(ApiTool.Backend.Data.AppDbContext db)
        => new LastAdminProtectionService(db);

    private async Task<Guid> SeedUserAsync(string email)
    {
        using var scope = OpenScope();
        var userId = Guid.NewGuid();
        scope.Db.Users.Add(new User { Id = userId, Email = email, CreatedAt = DateTime.UtcNow });
        await scope.Db.SaveChangesAsync();
        return userId;
    }

    private async Task<Guid> SeedOrgAsync(Guid ownerId, string slug, string name = "Test Org")
    {
        using var scope = OpenScope();
        var orgId = Guid.NewGuid();
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = name,
            Slug = slug,
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
        return orgId;
    }

    private async Task AddMemberAsync(Guid orgId, Guid userId, OrgRole role)
    {
        using var scope = OpenScope();
        scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
    }

    [Fact]
    public async Task User_with_no_owned_orgs_returns_empty_blocking_and_cascade()
    {
        var userId = await SeedUserAsync("noorg@example.com");

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(userId, default);

        blocking.Should().BeEmpty();
        cascade.Should().BeEmpty();
    }

    [Fact]
    public async Task Sole_owner_with_other_members_is_blocking()
    {
        var ownerId = await SeedUserAsync("owner@example.com");
        var memberId = await SeedUserAsync("member@example.com");
        var orgId = await SeedOrgAsync(ownerId, "blocked-org", "Blocked Org");
        await AddMemberAsync(orgId, ownerId, OrgRole.Owner);
        await AddMemberAsync(orgId, memberId, OrgRole.Member);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(ownerId, default);

        blocking.Should().ContainSingle(because: "sole owner with other members is blocking");
        cascade.Should().BeEmpty();
    }

    [Fact]
    public async Task Sole_owner_with_zero_members_is_cascade()
    {
        var ownerId = await SeedUserAsync("solo@example.com");
        var orgId = await SeedOrgAsync(ownerId, "empty-org", "Empty Org");
        await AddMemberAsync(orgId, ownerId, OrgRole.Owner);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(ownerId, default);

        blocking.Should().BeEmpty();
        cascade.Should().ContainSingle(because: "sole owner with zero other members is cascade");
    }

    [Fact]
    public async Task Owner_with_another_owner_is_neither_blocking_nor_cascade()
    {
        var ownerA = await SeedUserAsync("ownera@example.com");
        var ownerB = await SeedUserAsync("ownerb@example.com");
        var orgId = await SeedOrgAsync(ownerA, "dual-owner-org");
        await AddMemberAsync(orgId, ownerA, OrgRole.Owner);
        await AddMemberAsync(orgId, ownerB, OrgRole.Owner);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(ownerA, default);

        blocking.Should().BeEmpty(because: "org with another owner present should not block deletion");
        cascade.Should().BeEmpty(because: "org with another owner is not cascade — another owner takes over");
    }

    [Fact]
    public async Task Admin_only_in_org_is_neither()
    {
        var adminId = await SeedUserAsync("admin@example.com");
        var ownerId = await SeedUserAsync("owner@example.com");
        var orgId = await SeedOrgAsync(ownerId, "admin-org");
        await AddMemberAsync(orgId, ownerId, OrgRole.Owner);
        await AddMemberAsync(orgId, adminId, OrgRole.Admin);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(adminId, default);

        blocking.Should().BeEmpty(because: "admin-only membership should not block or cascade");
        cascade.Should().BeEmpty();
    }

    [Fact]
    public async Task Member_only_in_org_is_neither()
    {
        var memberId = await SeedUserAsync("member@example.com");
        var ownerId = await SeedUserAsync("owner@example.com");
        var orgId = await SeedOrgAsync(ownerId, "member-org");
        await AddMemberAsync(orgId, ownerId, OrgRole.Owner);
        await AddMemberAsync(orgId, memberId, OrgRole.Member);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(memberId, default);

        blocking.Should().BeEmpty(because: "regular member should not block or cascade");
        cascade.Should().BeEmpty();
    }

    [Fact]
    public async Task User_blocking_in_orgA_and_cascade_in_orgB_returns_both_lists()
    {
        var userId = await SeedUserAsync("mixed@example.com");
        var otherMember = await SeedUserAsync("other@example.com");

        // OrgA: userId is sole owner, but has another member → blocking
        var orgA = await SeedOrgAsync(userId, "blocking-org-a", "Blocking Org A");
        await AddMemberAsync(orgA, userId, OrgRole.Owner);
        await AddMemberAsync(orgA, otherMember, OrgRole.Member);

        // OrgB: userId is sole owner, no other members → cascade
        var orgB = await SeedOrgAsync(userId, "cascade-org-b", "Cascade Org B");
        await AddMemberAsync(orgB, userId, OrgRole.Owner);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, cascade) = await svc.ClassifyOwnedOrgsAsync(userId, default);

        blocking.Should().ContainSingle(because: "orgA is blocking");
        cascade.Should().ContainSingle(because: "orgB is cascade");
        cascade.Should().Contain(orgB);
    }

    [Fact]
    public async Task Blocking_orgs_include_slug_and_name()
    {
        var ownerId = await SeedUserAsync("slugowner@example.com");
        var memberId = await SeedUserAsync("slugmember@example.com");
        var orgId = await SeedOrgAsync(ownerId, "my-test-org", "My Test Org");
        await AddMemberAsync(orgId, ownerId, OrgRole.Owner);
        await AddMemberAsync(orgId, memberId, OrgRole.Member);

        using var scope = OpenScope();
        var svc = BuildService(scope.Db);
        var (blocking, _) = await svc.ClassifyOwnedOrgsAsync(ownerId, default);

        blocking.Should().ContainSingle();
        blocking[0].Slug.Should().Be("my-test-org");
        blocking[0].Name.Should().Be("My Test Org");
    }
}
