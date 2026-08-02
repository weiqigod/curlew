using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Data;

/// <summary>Verifies that EF Core migrations create the expected schema.</summary>
public sealed class AppDbContextSchemaTests
{
    [Theory]
    [InlineData("organizations")]
    [InlineData("organization_members")]
    [InlineData("organization_invitations")]
    [InlineData("organization_audit_log")]
    [InlineData("results")]
    [InlineData("result_items")]
    [InlineData("schedules")]
    [InlineData("scheduled_runs")]
    [InlineData("notification_rules")]
    [InlineData("notification_deliveries")]
    [InlineData("coordinator_jobs")]
    [InlineData("coordinator_shards")]
    [InlineData("signing_keys")]
    [InlineData("refresh_tokens")]
    [InlineData("trials")]
    [InlineData("team_vaults")]
    public async Task Migration_creates_expected_table(string tableName)
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var exists = await TableExistsAsync(db, tableName);
        exists.Should().BeTrue(because: $"migration should create table '{tableName}'");
    }

    [Fact]
    public async Task Organizations_slug_is_unique()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = "a@b.c", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = Guid.NewGuid(),
            Name = "Acme",
            Slug = "acme",
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        db.Organizations.Add(new Organization
        {
            Id = Guid.NewGuid(),
            Name = "Acme2",
            Slug = "acme",  // duplicate slug
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });

        var act = async () => await db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(because: "slug must be globally unique");
    }

    [Fact]
    public async Task Result_items_cascade_delete_when_parent_result_is_removed()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        // Seed a user and org (required by FK constraints)
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"cascade-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "CascadeOrg",
            Slug = $"cascade-{userId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        // Insert a result and two items
        var resultId = Guid.NewGuid();
        db.Results.Add(new Result
        {
            Id = resultId,
            OrgId = orgId,
            UploadedBy = userId,
            CollectionName = "cascade-test",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 2,
            FailCount = 0,
            SkippedCount = 0,
            CreatedAt = DateTime.UtcNow,
        });
        db.ResultItems.Add(new ResultItem
        {
            Id = Guid.NewGuid(),
            ResultId = resultId,
            Ordinal = 0,
            Name = "test-1",
            Status = ResultStatus.Passed,
            DurationMs = 50,
        });
        db.ResultItems.Add(new ResultItem
        {
            Id = Guid.NewGuid(),
            ResultId = resultId,
            Ordinal = 1,
            Name = "test-2",
            Status = ResultStatus.Passed,
            DurationMs = 50,
        });
        await db.SaveChangesAsync();

        // Delete the parent result
        db.ChangeTracker.Clear();
        var result = await db.Results.FindAsync(resultId);
        db.Results.Remove(result!);
        await db.SaveChangesAsync();

        // Items should be gone (cascade)
        var remainingItems = await db.ResultItems.CountAsync(i => i.ResultId == resultId);
        remainingItems.Should().Be(0, because: "cascade delete should remove child items");
    }

    [Fact]
    public async Task Schedules_name_is_unique_per_org()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"sched-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "SchedOrg",
            Slug = $"sched-{userId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var schedId1 = Guid.NewGuid();
        db.Schedules.Add(new Schedule
        {
            Id = schedId1,
            OrgId = orgId,
            Name = "nightly",
            CronExpression = "0 2 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = userId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ChangeTracker.Clear();
        db.Schedules.Add(new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Name = "nightly",  // duplicate name in same org
            CronExpression = "0 3 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = userId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });

        var act = async () => await db.SaveChangesAsync();
        await act.Should().ThrowAsync<Microsoft.EntityFrameworkCore.DbUpdateException>(
            because: "schedule name must be unique per org");
    }

    [Fact]
    public async Task Scheduled_runs_cascade_delete_when_parent_schedule_is_removed()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"cascade-sched-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "CascadeSchedOrg",
            Slug = $"casc-{userId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var schedId = Guid.NewGuid();
        db.Schedules.Add(new Schedule
        {
            Id = schedId,
            OrgId = orgId,
            Name = "cascadetest",
            CronExpression = "0 2 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = userId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedId,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        db.ScheduledRuns.Add(new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedId,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        // Delete the schedule — runs should cascade
        db.ChangeTracker.Clear();
        var sched = await db.Schedules.FindAsync(schedId);
        db.Schedules.Remove(sched!);
        await db.SaveChangesAsync();

        var remaining = await db.ScheduledRuns.CountAsync(r => r.ScheduleId == schedId);
        remaining.Should().Be(0, because: "cascade delete should remove child scheduled runs");
    }

    [Fact]
    public async Task NotificationRules_and_deliveries_tables_exist_with_expected_columns()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var ruleColumns = await ListTableColumnsAsync(scope.Connection, "notification_rules");
        ruleColumns.Should().Contain(new[] { "Id", "OrgId", "Channel", "Target", "OnEvents",
                                             "CreatedBy", "CreatedAt" });

        var deliveryColumns = await ListTableColumnsAsync(scope.Connection, "notification_deliveries");
        deliveryColumns.Should().Contain(new[] { "Id", "RuleId", "OrgId", "ResultId",
                                                 "Channel", "Status", "ResponseCode",
                                                 "AttemptCount", "ErrorMessage", "AttemptedAt" });
    }

    [Fact]
    public async Task SsoCredentials_has_table_and_unique_org_kid_index()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        // Verify table exists
        var exists = await TableExistsAsync(db, "sso_credentials");
        exists.Should().BeTrue(because: "migration should create the sso_credentials table");

        // Verify unique (org_id, kid) constraint
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"sso-cred-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "SsoOrg",
            Slug = $"sso-{userId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        db.SsoCredentials.Add(new ApiTool.Backend.Data.Entities.SsoCredential
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Kid = "kid-001",
            PublicCertPem = "-----BEGIN CERTIFICATE-----\nMIIBxTCCAW+gA\n-----END CERTIFICATE-----",
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        // Duplicate (OrgId, Kid) must fail
        db.ChangeTracker.Clear();
        db.SsoCredentials.Add(new ApiTool.Backend.Data.Entities.SsoCredential
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Kid = "kid-001",  // same kid for same org
            PublicCertPem = "-----BEGIN CERTIFICATE-----\nDIFFERENT\n-----END CERTIFICATE-----",
            CreatedAt = DateTime.UtcNow,
        });

        var act = async () => await db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "the (org_id, kid) composite index must be unique");
    }

    [Fact]
    public async Task User_password_hash_and_is_admin_round_trip()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var u = new User
        {
            Id = Guid.NewGuid(),
            Email = "bootstrap@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA==$aGFzaA==",
            IsAdmin = true,
        };
        scope.Db.Users.Add(u);
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        var back = await scope.Db.Users.SingleAsync(x => x.Id == u.Id);
        back.PasswordHash.Should().Be(u.PasswordHash);
        back.IsAdmin.Should().BeTrue();
    }

    [Fact]
    public async Task User_is_admin_defaults_false()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var u = new User { Id = Guid.NewGuid(), Email = $"plain-{Guid.NewGuid():N}@example.com", CreatedAt = DateTime.UtcNow };
        scope.Db.Users.Add(u);
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();
        (await scope.Db.Users.SingleAsync(x => x.Id == u.Id)).IsAdmin.Should().BeFalse();
    }

    [Fact]
    public async Task SigningKeys_only_one_current_at_a_time()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        scope.Db.SigningKeys.Add(MakeSigningKey("kid-a", KeyStatus.Current));
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        scope.Db.SigningKeys.Add(MakeSigningKey("kid-b", KeyStatus.Current));

        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "idx_signing_keys_current is a partial unique index on status='current'");
    }

    [Fact]
    public async Task SigningKeys_only_one_next_at_a_time()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        scope.Db.SigningKeys.Add(MakeSigningKey("kid-c", KeyStatus.Next));
        await scope.Db.SaveChangesAsync();

        scope.Db.ChangeTracker.Clear();
        scope.Db.SigningKeys.Add(MakeSigningKey("kid-d", KeyStatus.Next));

        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().ThrowAsync<DbUpdateException>(
            because: "idx_signing_keys_next is a partial unique index on status='next'");
    }

    [Fact]
    public async Task SigningKeys_multiple_verifying_keys_allowed()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        scope.Db.SigningKeys.Add(MakeSigningKey("kid-v1", KeyStatus.Verifying));
        scope.Db.SigningKeys.Add(MakeSigningKey("kid-v2", KeyStatus.Verifying));
        var act = async () => await scope.Db.SaveChangesAsync();
        await act.Should().NotThrowAsync(
            because: "multiple verifying keys should coexist without constraint violation");
    }

    private static SigningKey MakeSigningKey(string kid, string status) => new()
    {
        Kid = kid,
        Algorithm = "ES256",
        Status = status,
        PublicKeyJwkJson = "{}",
        CreatedAt = DateTime.UtcNow,
    };

    private static async Task<bool> TableExistsAsync(Microsoft.EntityFrameworkCore.DbContext db, string tableName)
    {
        await using var cmd = db.Database.GetDbConnection().CreateCommand();
        cmd.CommandText = "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=@tableName";
        var param = cmd.CreateParameter();
        param.ParameterName = "@tableName";
        param.Value = tableName;
        cmd.Parameters.Add(param);
        var result = await cmd.ExecuteScalarAsync();
        return Convert.ToInt64(result) > 0;
    }

    // ── M18-005: GDPR deletion state machine columns ─────────────────────────

    [Fact]
    public async Task Users_table_has_pending_deletion_at_and_anonymised_at_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var columns = await ListTableColumnsAsync(scope.Connection, "users");
        columns.Should().Contain("pending_deletion_at",
            because: "M18-005 adds users.pending_deletion_at (TIMESTAMPTZ NULL)");
        columns.Should().Contain("anonymised_at",
            because: "M18-005 adds users.anonymised_at (TIMESTAMPTZ NULL)");
    }

    [Fact]
    public async Task Deletion_reauth_tokens_table_exists_with_expected_columns()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var exists = await TableExistsAsync(scope.Db, "deletion_reauth_tokens");
        exists.Should().BeTrue(because: "M18-005 creates the deletion_reauth_tokens table");

        var columns = await ListTableColumnsAsync(scope.Connection, "deletion_reauth_tokens");
        columns.Should().Contain(["Id", "user_id", "token_hash", "issued_at", "expires_at", "consumed_at"],
            because: "deletion_reauth_tokens must mirror password_reset_tokens shape");
    }

    private static async Task<IReadOnlyList<string>> ListTableColumnsAsync(
        Microsoft.Data.Sqlite.SqliteConnection conn, string tableName)
    {
        var columns = new List<string>();
        await using var cmd = conn.CreateCommand();
        cmd.CommandText = $"PRAGMA table_info({tableName})";
        await using var reader = await cmd.ExecuteReaderAsync();
        while (await reader.ReadAsync())
        {
            columns.Add(reader.GetString(1)); // column 1 = name
        }
        return columns;
    }
}
