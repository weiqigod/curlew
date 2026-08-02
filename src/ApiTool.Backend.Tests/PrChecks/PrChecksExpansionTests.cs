// Refs docs/SPECIFICATION.md:8429-8550 (pr_checks v4.2.1 schema and lifecycle).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Migration smoke tests for the pr_checks table expansion (migration 0017_PrChecksV421).
/// Uses a real SQLite in-memory database via TestDb so constraint enforcement is real.
/// Named PrChecksExpansion so the filter FullyQualifiedName~PrChecksExpansion matches.
/// </summary>
public sealed class PrChecksExpansionTests
{
    // ── Seed helpers ──────────────────────────────────────────────────────────

    private static async Task<(Guid userId, Guid orgId)> SeedOrgAsync(AppDbContext db, string email, string slug)
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = email, CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = slug, Slug = slug, OwnerId = userId,
            Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (userId, orgId);
    }

    private static PrCheck MakePrCheck(Guid orgId, string repo = "owner/repo", Guid? externalId = null) =>
        new()
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Repo = repo,
            Pr = 1,
            State = "success",
            CreatedAt = DateTime.UtcNow,
            ExternalId = externalId ?? Guid.NewGuid(),
        };

    // ── Migration schema tests ─────────────────────────────────────────────

    [Fact]
    public async Task Migration_AddsAllNewColumns_BackfillsExternalIdAndStatus()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "migration-test@example.com", "migration-test-org");
        var row = MakePrCheck(orgId);
        db.PrChecks.Add(row);
        await db.SaveChangesAsync();

        // Re-fetch and assert defaults
        var refetched = await db.PrChecks.AsNoTracking().FirstAsync(x => x.Id == row.Id);
        refetched.ExternalId.Should().NotBe(Guid.Empty);
        refetched.AttemptCount.Should().Be(0);
        refetched.Status.Should().Be("pending");
        refetched.HeadSha.Should().Be(string.Empty); // default backfill
        refetched.InstallationId.Should().BeNull();
        refetched.CheckRunId.Should().BeNull();
        refetched.Conclusion.Should().BeNull();
        refetched.PostedAt.Should().BeNull();
    }

    [Fact]
    public async Task Migration_ExternalId_Unique_RejectsDuplicates()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "unique-test@example.com", "unique-test-org");
        var fixedId = Guid.NewGuid();

        db.PrChecks.Add(MakePrCheck(orgId, externalId: fixedId));
        await db.SaveChangesAsync();

        db.PrChecks.Add(MakePrCheck(orgId, externalId: fixedId));
        var act = async () => await db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>();
    }

    [Fact]
    public async Task Migration_AllV421Columns_RoundTripSuccessfully()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var (_, orgId) = await SeedOrgAsync(db, "roundtrip@example.com", "roundtrip-org");
        var now = DateTime.UtcNow;
        var row = new PrCheck
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Repo = "owner/repo",
            Pr = 42,
            State = "success",
            CreatedAt = now,
            ExternalId = Guid.NewGuid(),
            HeadSha = "abc123def456abc123def456abc123def456abc1",
            InstallationId = 999L,
            CheckRunId = 12345L,
            Conclusion = "success",
            DetailsUrl = "https://example.com/runs/1",
            OutputSummary = "All tests passed",
            OutputText = "Detailed output here",
            AnnotationsJson = "[]",
            PostingStartedAt = now,
            PostedAt = now,
            AttemptCount = 1,
            LastError = null,
            Status = "posted",
        };

        db.PrChecks.Add(row);
        await db.SaveChangesAsync();

        var refetched = await db.PrChecks.AsNoTracking().FirstAsync(x => x.Id == row.Id);
        refetched.ExternalId.Should().Be(row.ExternalId);
        refetched.HeadSha.Should().Be("abc123def456abc123def456abc123def456abc1");
        refetched.InstallationId.Should().Be(999L);
        refetched.CheckRunId.Should().Be(12345L);
        refetched.Conclusion.Should().Be("success");
        refetched.DetailsUrl.Should().Be("https://example.com/runs/1");
        refetched.OutputSummary.Should().Be("All tests passed");
        refetched.PostedAt.Should().NotBeNull();
        refetched.AttemptCount.Should().Be(1);
        refetched.Status.Should().Be("posted");
    }
}
