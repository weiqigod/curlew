// Tests for gitlab_installations and gitlab_webhook_events schema (M16-013).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Data;

/// <summary>
/// Verifies the gitlab_installations and gitlab_webhook_events schema:
/// column presence, unique indexes, FK cascade behaviour.
/// Uses SQLite in-memory via EnsureCreated (not migrations — the schema tests in
/// AppDbContextSchemaTests cover migration application; these tests cover model shape).
/// Refs docs/SPECIFICATION.md:10893-10943.
/// </summary>
public sealed class GitLabSchemaTests
{
    private static async Task<(AppDbContext ctx, Guid orgId, Guid userId)> CreateContextWithOrgAsync()
    {
        var scope = TestDb.CreateOpen();
        var ctx = scope.Db;
        await ctx.Database.EnsureCreatedAsync();

        var userId = Guid.NewGuid();
        ctx.Users.Add(new User { Id = userId, Email = $"gl-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var orgId = Guid.NewGuid();
        ctx.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "GitLabTest",
            Slug = $"gl-test-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await ctx.SaveChangesAsync();

        return (ctx, orgId, userId);
    }

    private static GitLabInstallation MakeInstallation(Guid orgId, long projectId, string baseUrl) =>
        new()
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ProjectId = projectId,
            ProjectPath = $"group/project-{projectId}",
            GitLabBaseUrl = baseUrl,
            AccessTokenCiphertext = new byte[] { 1, 2, 3 },
            AccessTokenKid = "gitlab-kek-file-v1",
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };

    private static GitLabWebhookEvent MakeEvent(Guid installationId, string uuid) =>
        new()
        {
            Id = Guid.NewGuid(),
            EventUuid = uuid,
            EventType = "Pipeline Hook",
            InstallationId = installationId,
            ReceivedAt = DateTime.UtcNow,
            PayloadJson = "{}",
        };

    [Fact]
    public async Task GitlabInstallations_table_exists_with_required_columns()
    {
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            var row = MakeInstallation(orgId, 1234L, "https://gitlab.com");
            ctx.GitLabInstallations.Add(row);
            await ctx.SaveChangesAsync();

            var read = await ctx.GitLabInstallations.SingleAsync(x => x.Id == row.Id);
            read.ProjectId.Should().Be(1234L);
            read.AccessTokenCiphertext.Should().Equal(1, 2, 3);
            read.GitLabBaseUrl.Should().Be("https://gitlab.com");
        }
    }

    [Fact]
    public async Task Insert_duplicate_org_project_baseurl_violates_unique_index()
    {
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            ctx.GitLabInstallations.Add(MakeInstallation(orgId, 1234L, "https://gitlab.com"));
            await ctx.SaveChangesAsync();

            ctx.ChangeTracker.Clear();
            ctx.GitLabInstallations.Add(MakeInstallation(orgId, 1234L, "https://gitlab.com"));
            var act = async () => await ctx.SaveChangesAsync();
            await act.Should().ThrowAsync<DbUpdateException>();
        }
    }

    [Fact]
    public async Task Same_project_id_on_different_baseurl_is_allowed()
    {
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            ctx.GitLabInstallations.Add(MakeInstallation(orgId, 1234L, "https://gitlab.com"));
            ctx.GitLabInstallations.Add(MakeInstallation(orgId, 1234L, "https://gitlab.example.com"));
            await ctx.SaveChangesAsync();

            (await ctx.GitLabInstallations.CountAsync()).Should().Be(2);
        }
    }

    [Fact]
    public async Task Soft_deleted_row_does_not_block_new_insert_with_same_unique_tuple()
    {
        // Note: SQLite in-memory via EnsureCreated uses a partial unique index
        // (WHERE deleted_at IS NULL), so a soft-deleted row should not block re-insert.
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            var first = MakeInstallation(orgId, 1234L, "https://gitlab.com");
            first.DeletedAt = DateTime.UtcNow;
            ctx.GitLabInstallations.Add(first);
            await ctx.SaveChangesAsync();

            ctx.ChangeTracker.Clear();
            ctx.GitLabInstallations.Add(MakeInstallation(orgId, 1234L, "https://gitlab.com"));
            await ctx.SaveChangesAsync();  // Should succeed — partial index

            (await ctx.GitLabInstallations.CountAsync()).Should().Be(2);
        }
    }

    [Fact]
    public async Task Webhook_event_uuid_is_unique()
    {
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            var inst = MakeInstallation(orgId, 1234L, "https://gitlab.com");
            ctx.GitLabInstallations.Add(inst);
            await ctx.SaveChangesAsync();

            ctx.GitLabWebhookEvents.Add(MakeEvent(inst.Id, "uuid-1"));
            await ctx.SaveChangesAsync();

            ctx.ChangeTracker.Clear();
            ctx.GitLabWebhookEvents.Add(MakeEvent(inst.Id, "uuid-1"));
            var act = async () => await ctx.SaveChangesAsync();
            await act.Should().ThrowAsync<DbUpdateException>();
        }
    }

    [Fact]
    public async Task Hard_deleting_installation_cascades_to_webhook_events()
    {
        var (ctx, orgId, _) = await CreateContextWithOrgAsync();
        await using (ctx)
        {
            var inst = MakeInstallation(orgId, 1234L, "https://gitlab.com");
            ctx.GitLabInstallations.Add(inst);
            ctx.GitLabWebhookEvents.Add(MakeEvent(inst.Id, "uuid-x"));
            await ctx.SaveChangesAsync();

            ctx.GitLabInstallations.Remove(inst);
            await ctx.SaveChangesAsync();

            (await ctx.GitLabWebhookEvents.CountAsync()).Should().Be(0);
        }
    }
}
