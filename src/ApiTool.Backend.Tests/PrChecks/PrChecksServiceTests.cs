using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.PrChecks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>Tests for <see cref="PrChecksService"/> against in-memory SQLite.</summary>
public sealed class PrChecksServiceTests
{
    // ── Test helpers ──────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, PrChecksService svc, Guid userId, Guid orgId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"user-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var svc = new PrChecksService(db, TimeProvider.System);
        return (scope, db, svc, userId, orgId);
    }

    private static PrCheckRequest ValidRequest(string state = "success") => new()
    {
        Repo = "acme/api",
        Pr = 7,
        State = state,
    };

    // ── PostAsync happy path ──────────────────────────────────────────────────

    [Fact]
    public async Task PostAsync_persists_check_and_returns_dto()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, message) = await svc.PostAsync(userId, orgId, ValidRequest(), default);

            error.Should().Be(PrCheckError.None);
            message.Should().BeNull();
            dto.Should().NotBeNull();
            dto!.Id.Should().StartWith("prc_");
            dto.Repo.Should().Be("acme/api");
            dto.Pr.Should().Be(7);
            dto.State.Should().Be("success");
            dto.ResultId.Should().BeNull();

            var count = await db.PrChecks.CountAsync();
            count.Should().Be(1);
        }
    }

    [Fact]
    public async Task PostAsync_failure_state_is_persisted()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var (dto, error, _) = await svc.PostAsync(userId, orgId, ValidRequest("failure"), default);

            error.Should().Be(PrCheckError.None);
            dto!.State.Should().Be("failure");
        }
    }

    // ── PostAsync RBAC ────────────────────────────────────────────────────────

    [Fact]
    public async Task PostAsync_returns_PermissionDenied_for_non_member()
    {
        var (scope, _, svc, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var stranger = Guid.NewGuid();
            var (dto, error, _) = await svc.PostAsync(stranger, orgId, ValidRequest(), default);

            error.Should().Be(PrCheckError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    // ── PostAsync validation ──────────────────────────────────────────────────

    [Theory]
    [InlineData("pending")]
    [InlineData("unknown")]
    [InlineData("")]
    public async Task PostAsync_returns_InvalidState_for_bad_state(string state)
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = ValidRequest(state);
            var (dto, error, message) = await svc.PostAsync(userId, orgId, req, default);

            error.Should().Be(PrCheckError.InvalidState);
            dto.Should().BeNull();
            message.Should().Contain("state");
        }
    }

    [Fact]
    public async Task PostAsync_returns_InvalidState_for_missing_repo()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new PrCheckRequest { Pr = 1, State = "success" }; // no Repo
            var (dto, error, _) = await svc.PostAsync(userId, orgId, req, default);

            error.Should().Be(PrCheckError.InvalidState);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task PostAsync_returns_InvalidState_for_zero_pr()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new PrCheckRequest { Repo = "x/y", Pr = 0, State = "success" };
            var (dto, error, _) = await svc.PostAsync(userId, orgId, req, default);

            error.Should().Be(PrCheckError.InvalidState);
            dto.Should().BeNull();
        }
    }

    // ── ListAsync ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task ListAsync_returns_checks_newest_first()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            await svc.PostAsync(userId, orgId, ValidRequest("success"), default);
            await svc.PostAsync(userId, orgId, ValidRequest("failure"), default);

            var (checks, error) = await svc.ListAsync(userId, orgId, 10, default);

            error.Should().Be(PrCheckError.None);
            checks.Should().HaveCount(2);
            // Newest first: the second post (failure) should appear first.
            checks[0].State.Should().Be("failure");
        }
    }

    [Fact]
    public async Task ListAsync_returns_PermissionDenied_for_non_member()
    {
        var (scope, _, svc, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var stranger = Guid.NewGuid();
            var (checks, error) = await svc.ListAsync(stranger, orgId, 10, default);

            error.Should().Be(PrCheckError.PermissionDenied);
            checks.Should().BeEmpty();
        }
    }

    // ── ListAsync: additive posted_at + check_run_id fields (M14-021) ────────

    [Fact]
    public async Task ListAsync_returns_posted_at_and_check_run_id_when_set()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;
            db.PrChecks.Add(new ApiTool.Backend.Data.Entities.PrCheck
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Repo = "acme/api",
                Pr = 7,
                State = "success",
                CreatedAt = now,
                ExternalId = Guid.NewGuid(),
                HeadSha = new string('a', 40),
                PostedAt = now.AddSeconds(1),
                CheckRunId = 999_001L,
                Status = "posted",
            });
            await db.SaveChangesAsync();

            var (checks, error) = await svc.ListAsync(userId, orgId, 10, default);

            error.Should().Be(PrCheckError.None);
            checks.Should().HaveCount(1);
            checks[0].PostedAt.Should().NotBeNull();
            checks[0].CheckRunId.Should().Be(999_001L);
        }
    }

    [Fact]
    public async Task ListAsync_returns_null_posted_at_for_in_flight_check()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var now = DateTime.UtcNow;
            db.PrChecks.Add(new ApiTool.Backend.Data.Entities.PrCheck
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Repo = "acme/api",
                Pr = 9,
                State = "success",
                CreatedAt = now,
                ExternalId = Guid.NewGuid(),
                HeadSha = new string('b', 40),
                PostingStartedAt = now,
                PostedAt = null,
                CheckRunId = null,
                Status = "posting",
            });
            await db.SaveChangesAsync();

            var (checks, error) = await svc.ListAsync(userId, orgId, 10, default);

            error.Should().Be(PrCheckError.None);
            checks.Should().HaveCount(1);
            checks[0].PostedAt.Should().BeNull();
            checks[0].CheckRunId.Should().BeNull();
        }
    }
}
