using ApiTool.Backend.Audit;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>Verifies OrganizationService business rules against in-memory SQLite.</summary>
public sealed class OrganizationServiceTests
{
    private static async Task<(TestDbScope scope, ApiTool.Backend.Data.AppDbContext db, OrganizationService svc, Guid userId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = "test@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();

        var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
        var svc = new OrganizationService(db, TimeProvider.System, auditWriter, new TierGate(db));
        return (scope, db, svc, userId);
    }

    [Fact]
    public async Task Create_inserts_org_member_and_returns_owner_role()
    {
        var (scope, db, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, "Acme", "acme", default);

            error.Should().Be(OrgError.None);
            dto.Should().NotBeNull();
            dto!.Role.Should().Be("owner");
            dto.SeatCount.Should().Be(1);
            dto.SeatLimit.Should().Be(OrganizationService.FreeTierSeatLimit);
            dto.Tier.Should().Be("free");

            var member = await db.OrganizationMembers.SingleOrDefaultAsync();
            member.Should().NotBeNull();
            member!.Role.Should().Be(OrgRole.Owner);
        }
    }

    [Fact]
    public async Task Create_rejects_uppercase_slug_with_invalid_slug_error()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, "Acme", "Acme", default);

            error.Should().Be(OrgError.InvalidSlug);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_rejects_duplicate_slug_with_slug_taken_error()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, "Acme", "acme", default);
            var (dto, error, _) = await svc.CreateAsync(userId, "Acme2", "acme", default);

            error.Should().Be(OrgError.SlugTaken);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_persists_audit_log_entry_org_created()
    {
        var (scope, db, svc, userId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, "Acme", "acme", default);

            var log = await db.OrganizationAuditLog.SingleOrDefaultAsync();
            log.Should().NotBeNull();
            log!.EventType.Should().Be("org.created");
        }
    }

    [Fact]
    public async Task Create_persists_valid_json_in_audit_log_for_special_character_names()
    {
        var (scope, db, svc, userId) = await BuildAsync();
        await using (scope)
        {
            // Org name contains a double-quote — would produce malformed JSON via string interpolation.
            await svc.CreateAsync(userId, "test\"corp", "testcorp", default);

            var log = await db.OrganizationAuditLog.SingleOrDefaultAsync();
            log.Should().NotBeNull();
            // Verify the stored payload is valid JSON and the name is correctly escaped.
            var parsed = System.Text.Json.JsonDocument.Parse(log!.PayloadJson);
            parsed.RootElement.GetProperty("name").GetString().Should().Be("test\"corp");
        }
    }

    [Fact]
    public async Task List_returns_empty_when_user_has_no_memberships()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var list = await svc.ListForUserAsync(userId, default);

            list.Should().BeEmpty();
        }
    }

    [Fact]
    public async Task List_returns_orgs_with_role_and_seat_count()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            await svc.CreateAsync(userId, "Acme", "acme", default);

            var list = await svc.ListForUserAsync(userId, default);

            list.Should().HaveCount(1);
            list[0].Role.Should().Be("owner");
            list[0].SeatCount.Should().Be(1);
        }
    }

    [Fact]
    public async Task List_and_get_return_current_subscription_tier()
    {
        var (scope, db, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (created, _, _) = await svc.CreateAsync(userId, "Team Org", "team-org", default);
            var orgId = Guid.Parse(OrgId.ToGuidString(created!.Id));
            db.Subscriptions.Add(new Subscription
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Tier = SubscriptionTier.Team,
                Status = SubscriptionStatus.Active,
                SeatCount = 1,
                SeatLimit = 10,
                CurrentPeriodStart = DateTime.UtcNow,
                CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var list = await svc.ListForUserAsync(userId, default);
            var (detail, error) = await svc.GetByIdForUserAsync(userId, orgId, default);

            list.Should().ContainSingle().Which.Tier.Should().Be("team");
            error.Should().Be(OrgError.None);
            detail!.Tier.Should().Be("team");
        }
    }

    [Fact]
    public async Task GetById_returns_not_found_when_user_is_not_member()
    {
        var (scope, db, svc, userId) = await BuildAsync();
        await using (scope)
        {
            // Create a separate user and org they are not a member of
            var otherUserId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherUserId, Email = "other@example.com", CreatedAt = DateTime.UtcNow });
            await db.SaveChangesAsync();

            var (dto, _, _) = await svc.CreateAsync(otherUserId, "Other Org", "other-org", default);
            var orgId = Guid.Parse(OrgId.ToGuidString(dto!.Id));

            var (result, error) = await svc.GetByIdForUserAsync(userId, orgId, default);

            error.Should().Be(OrgError.NotFound);
            result.Should().BeNull();
        }
    }

    [Fact]
    public async Task GetById_returns_org_for_member()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (created, _, _) = await svc.CreateAsync(userId, "Acme", "acme", default);
            var orgId = Guid.Parse(OrgId.ToGuidString(created!.Id));

            var (dto, error) = await svc.GetByIdForUserAsync(userId, orgId, default);

            error.Should().Be(OrgError.None);
            dto.Should().NotBeNull();
            dto!.Slug.Should().Be("acme");
            dto.Role.Should().Be("owner");
        }
    }

    [Fact]
    public async Task Create_rejects_empty_name_with_invalid_name_error()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, "", "valid-slug", default);

            error.Should().Be(OrgError.InvalidName);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_rejects_whitespace_name_with_invalid_name_error()
    {
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, "   ", "valid-slug", default);

            error.Should().Be(OrgError.InvalidName);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_rejects_name_exceeding_max_length()
    {
        // 101 characters — one over the 100-character maximum.
        var longName = new string('a', 101);
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, longName, "valid-slug", default);

            error.Should().Be(OrgError.InvalidName, because: "names longer than 100 characters should be rejected");
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task Create_accepts_name_at_max_length()
    {
        // Exactly 100 characters — on the boundary.
        var maxName = new string('a', 100);
        var (scope, _, svc, userId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.CreateAsync(userId, maxName, "valid-slug", default);

            error.Should().Be(OrgError.None, because: "a name of exactly 100 characters should be accepted");
            dto.Should().NotBeNull();
        }
    }
}
