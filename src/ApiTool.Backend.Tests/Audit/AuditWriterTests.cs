using ApiTool.Backend.Audit;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>Unit tests for <see cref="AuditWriter"/> against in-memory SQLite.</summary>
public sealed class AuditWriterTests
{
    private static async Task<(AuditWriter writer, TestDbScope scope)> BuildAsync(
        string? ipAddress = null, string? userAgent = null)
    {
        var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        var auditCtx = new AuditContext { IpAddress = ipAddress, UserAgent = userAgent };
        var writer = new AuditWriter(scope.Db, auditCtx, TimeProvider.System);
        return (writer, scope);
    }

    [Fact]
    public async Task Append_writes_row_with_ip_and_user_agent_from_context()
    {
        var (writer, scope) = await BuildAsync("10.0.0.1", "TestAgent/1.0");
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        writer.Append(new AuditEvent(orgId, Guid.NewGuid(), "test.event"));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.IpAddress.Should().Be("10.0.0.1");
        entry.UserAgent.Should().Be("TestAgent/1.0");
    }

    [Fact]
    public async Task Append_serializes_previous_and_new_state_when_provided()
    {
        var (writer, scope) = await BuildAsync();
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        writer.Append(new AuditEvent(
            orgId, Guid.NewGuid(), "org.settings.updated",
            PreviousState: new { name = "Old Name" },
            NewState: new { name = "New Name" }));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.PreviousStateJson.Should().Contain("Old Name");
        entry.NewStateJson.Should().Contain("New Name");
    }

    [Fact]
    public async Task Append_records_success_false_with_failure_reason()
    {
        var (writer, scope) = await BuildAsync();
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        writer.Append(new AuditEvent(
            orgId, Guid.Empty, "sso.login",
            Success: false, FailureReason: "user_not_found"));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.Success.Should().BeFalse();
        entry.FailureReason.Should().Be("user_not_found");
    }

    [Fact]
    public async Task Append_serializes_payload_with_snake_case()
    {
        var (writer, scope) = await BuildAsync();
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        writer.Append(new AuditEvent(
            orgId, Guid.NewGuid(), "member.invited",
            Payload: new { EmailAddress = "test@example.com", RoleType = "member" }));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.PayloadJson.Should().Contain("email_address");
        entry.PayloadJson.Should().Contain("role_type");
    }

    [Fact]
    public async Task Append_stores_target_type_and_id()
    {
        var (writer, scope) = await BuildAsync();
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        var targetId = Guid.NewGuid();
        writer.Append(new AuditEvent(
            orgId, Guid.NewGuid(), "member.invited",
            TargetType: "invitation", TargetId: targetId));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.TargetType.Should().Be("invitation");
        entry.TargetId.Should().Be(targetId);
    }

    [Fact]
    public async Task Append_defaults_success_to_true()
    {
        var (writer, scope) = await BuildAsync();
        await using var _ = scope;

        var orgId = await SetupOrgAsync(scope);
        writer.Append(new AuditEvent(orgId, Guid.NewGuid(), "org.created"));
        await scope.Db.SaveChangesAsync();

        var entry = await scope.Db.OrganizationAuditLog.FirstAsync(e => e.OrgId == orgId);
        entry.Success.Should().BeTrue();
    }

    private static async Task<Guid> SetupOrgAsync(TestDbScope scope)
    {
        var ownerId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        scope.Db.Users.Add(new ApiTool.Backend.Data.Entities.User
        {
            Id = ownerId, Email = $"aw-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow,
        });
        scope.Db.Organizations.Add(new ApiTool.Backend.Data.Entities.Organization
        {
            Id = orgId, Name = "AuditWriterOrg", Slug = $"aw{orgId:N}"[..20],
            OwnerId = ownerId, Status = ApiTool.Backend.Data.Entities.OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();
        return orgId;
    }
}
