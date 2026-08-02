using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Data.GdprAttributes;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>Tests for GdprAttributeScanner and GdprBundleManifest.</summary>
public class GdprAttributeScannerTests
{
    private static readonly GdprBundleManifest Manifest =
        GdprAttributeScanner.Scan(typeof(AppDbContext).Assembly);

    [Fact]
    public void Manifest_contains_exactly_thirteen_tables() =>
        Manifest.Entries.Should().HaveCount(14, because: "M18-005 adds deletion_reauth_tokens");

    [Theory]
    [InlineData(typeof(User), GdprDisposition.InExportInDeletionHard)]
    [InlineData(typeof(RefreshToken), GdprDisposition.InExportInDeletionHard)]
    [InlineData(typeof(OrganizationMember), GdprDisposition.InExportInDeletionHard)]
    [InlineData(typeof(EmailVerificationToken), GdprDisposition.InExportInDeletionHard)]
    [InlineData(typeof(PasswordResetToken), GdprDisposition.InExportInDeletionHard)]
    [InlineData(typeof(OrganizationAuditLogEntry), GdprDisposition.InExportInDeletionAnonymise)]
    [InlineData(typeof(NotificationRule), GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [InlineData(typeof(CustomRole), GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [InlineData(typeof(Schedule), GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [InlineData(typeof(CoordinatorJob), GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [InlineData(typeof(TeamVault), GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [InlineData(typeof(GithubInstallation), GdprDisposition.ExcludedFromBoth)]
    [InlineData(typeof(GitLabInstallation), GdprDisposition.ExcludedFromBoth)]
    public void Manifest_records_expected_disposition(Type entity, GdprDisposition expected) =>
        Manifest.FindByEntity(entity)!.Disposition.Should().Be(expected);

    [Fact]
    public void InExport_subset_has_seven_entries() =>
        Manifest.InExport.Should().HaveCount(7, because: "M18-005 adds deletion_reauth_tokens — 6 hard-delete + 1 anonymise = 7");

    [Fact]
    public void TablesAnonymisedOnDeletion_includes_audit_log_and_creators() =>
        Manifest.TablesAnonymisedOnDeletion
            .Select(e => e.EntityType)
            .Should().BeEquivalentTo(new[]
            {
                typeof(OrganizationAuditLogEntry),
                typeof(NotificationRule),
                typeof(CustomRole),
                typeof(Schedule),
                typeof(CoordinatorJob),
                typeof(TeamVault),
            });

    [Fact]
    public void ColumnsToAnonymise_includes_OrganizationMember_InvitedBy_despite_hard_table_disposition()
    {
        // OrganizationMember's table-level disposition is InExportInDeletionHard (because UserId is
        // hard-deleted), but its InvitedBy column carries [GdprAnonymise(SetNull)]. The column-level
        // flattener must surface it for M18-006.
        Manifest.ColumnsToAnonymise.Should().Contain(p =>
            p.Entry.EntityType == typeof(OrganizationMember)
            && p.Column.PropertyName == nameof(OrganizationMember.InvitedBy)
            && p.Column.AnonymiseAs == AnonymiseAs.SetNull);
    }

    [Fact]
    public void ColumnsToAnonymise_surfaces_audit_log_token_and_null_columns()
    {
        var auditCols = Manifest.ColumnsToAnonymise
            .Where(p => p.Entry.EntityType == typeof(OrganizationAuditLogEntry))
            .Select(p => (p.Column.PropertyName, p.Column.AnonymiseAs))
            .ToList();

        auditCols.Should().Contain((nameof(OrganizationAuditLogEntry.ActorId), AnonymiseAs.SetNull));
        auditCols.Should().Contain((nameof(OrganizationAuditLogEntry.ActorEmail), AnonymiseAs.DeletedUserToken));
    }

    [Fact]
    public void Audit_log_actor_id_anonymises_to_set_null()
    {
        var entry = Manifest.FindByEntity(typeof(OrganizationAuditLogEntry))!;
        var actorId = entry.Columns.Single(c => c.PropertyName == nameof(OrganizationAuditLogEntry.ActorId));
        actorId.AnonymiseAs.Should().Be(AnonymiseAs.SetNull);
    }

    [Fact]
    public void Audit_log_actor_email_anonymises_to_deleted_user_token()
    {
        var entry = Manifest.FindByEntity(typeof(OrganizationAuditLogEntry))!;
        var actorEmail = entry.Columns.Single(c => c.PropertyName == nameof(OrganizationAuditLogEntry.ActorEmail));
        actorEmail.AnonymiseAs.Should().Be(AnonymiseAs.DeletedUserToken);
    }

    [Fact]
    public void Audit_log_actor_email_column_disposition_is_InExportInDeletionAnonymise()
    {
        // ActorEmail carries [GdprAnonymise] but no [GdprIncluded]; the scanner's fallback
        // default assigns InExportInDeletionAnonymise to such columns. Pins the fallback so
        // flipping it would fail loudly.
        var entry = Manifest.FindByEntity(typeof(OrganizationAuditLogEntry))!;
        var actorEmail = entry.Columns.Single(c => c.PropertyName == nameof(OrganizationAuditLogEntry.ActorEmail));
        actorEmail.Disposition.Should().Be(GdprDisposition.InExportInDeletionAnonymise);
    }

    [Fact]
    public void Lazy_Manifest_static_returns_fourteen_entries() =>
        GdprAttributeScanner.Manifest.Entries.Should().HaveCount(14, because: "M18-005 adds deletion_reauth_tokens");

    [Fact]
    public void SetNull_columns_are_widened_to_nullable_Guid()
    {
        // M18-006: all SetNull columns have been widened to Guid? so the anonymiser can NULL them.
        var widened = new (Type Entity, string Property)[]
        {
            (typeof(OrganizationAuditLogEntry), nameof(OrganizationAuditLogEntry.ActorId)),
            (typeof(NotificationRule), nameof(NotificationRule.CreatedBy)),
            (typeof(CustomRole), nameof(CustomRole.CreatedBy)),
            (typeof(Schedule), nameof(Schedule.CreatedBy)),
            (typeof(CoordinatorJob), nameof(CoordinatorJob.CreatedBy)),
            (typeof(TeamVault), nameof(TeamVault.CreatedBy)),
            (typeof(TeamVault), nameof(TeamVault.UpdatedBy)),
        };

        foreach (var (entity, property) in widened)
            entity.GetProperty(property)!.PropertyType.Should().Be(typeof(Guid?),
                $"{entity.Name}.{property} must be widened to Guid? for [GdprAnonymise(SetNull)] to be honoured");
    }

    [Fact]
    public void Scan_is_deterministic()
    {
        var second = GdprAttributeScanner.Scan(typeof(AppDbContext).Assembly);
        second.Entries.Should().BeEquivalentTo(Manifest.Entries, opts => opts.WithStrictOrdering());
    }

    [Fact]
    public void Scan_throws_ArgumentNullException_when_assembly_is_null() =>
        FluentActions.Invoking(() => GdprAttributeScanner.Scan(null!))
            .Should().Throw<ArgumentNullException>()
            .WithParameterName("assembly");

    [Fact]
    public void Table_names_match_AppDbContext_ToTable_calls()
    {
        // Loop over every manifest entry and verify its TableName matches the EF model's
        // ToTable(...) call for the corresponding entity type. Catches drift when a future
        // AppDbContext.OnModelCreating rename desynchronises from the scanner's TableNameOf map.
        var opts = new DbContextOptionsBuilder<AppDbContext>()
            .UseInMemoryDatabase($"gdpr_tablename_check_{Guid.NewGuid():N}")
            .Options;
        using var ctx = new AppDbContext(opts);

        foreach (var entry in Manifest.Entries)
        {
            var efType = ctx.Model.FindEntityType(entry.EntityType);
            efType.Should().NotBeNull(
                $"entity '{entry.EntityType.Name}' should be mapped in AppDbContext");
            var efTableName = efType!.GetTableName();
            entry.TableName.Should().Be(efTableName,
                $"scanner's TableNameOf for '{entry.EntityType.Name}' must match EF model");
        }
    }

    [Fact]
    public void ExcludedFromBoth_contains_exactly_GithubInstallation_and_GitLabInstallation()
    {
        var excluded = Manifest.ExcludedFromBoth.ToList();
        excluded.Should().HaveCount(2);
        excluded.Select(e => e.EntityType).Should().BeEquivalentTo(new[]
        {
            typeof(GithubInstallation),
            typeof(GitLabInstallation),
        });
        excluded.Should().OnlyContain(e => e.Columns.Count == 0);
    }

    [Fact]
    public void InExport_does_not_contain_excluded_tables() =>
        Manifest.InExport.Should().NotContain(e =>
            e.Disposition == GdprDisposition.ExcludedFromBoth
            || e.Disposition == GdprDisposition.ExcludedFromExportAnonymisedInDeletion);

    [Fact]
    public void TeamVault_has_two_column_entries()
    {
        var entry = Manifest.FindByEntity(typeof(TeamVault))!;
        entry.Columns.Should().HaveCount(2);
        entry.Columns.Select(c => c.PropertyName).Should().BeEquivalentTo(
            new[] { nameof(TeamVault.CreatedBy), nameof(TeamVault.UpdatedBy) });
    }

    [Fact]
    public void OrganizationMember_InvitedBy_is_anonymisable()
    {
        var entry = Manifest.FindByEntity(typeof(OrganizationMember))!;
        var invitedBy = entry.Columns.Single(c => c.PropertyName == nameof(OrganizationMember.InvitedBy));
        invitedBy.AnonymiseAs.Should().Be(AnonymiseAs.SetNull);
        invitedBy.Disposition.Should().Be(GdprDisposition.ExcludedFromExportAnonymisedInDeletion);
    }

    [Fact]
    public void Entries_are_ordered_alphabetically_by_entity_name()
    {
        var names = Manifest.Entries.Select(e => e.EntityType.Name).ToList();
        names.Should().BeInAscendingOrder(StringComparer.Ordinal);
    }

    // ── M18-004: UserIdColumnName on InExport entries ─────────────────────

    [Theory]
    [InlineData(typeof(User), "Id")]
    [InlineData(typeof(RefreshToken), "UserId")]
    [InlineData(typeof(OrganizationMember), "UserId")]
    [InlineData(typeof(EmailVerificationToken), "UserId")]
    [InlineData(typeof(PasswordResetToken), "UserId")]
    [InlineData(typeof(OrganizationAuditLogEntry), "ActorId")]
    public void InExport_entries_carry_UserIdColumnName(Type entity, string expected) =>
        Manifest.FindByEntity(entity)!.UserIdColumnName.Should().Be(expected);

    [Fact]
    public void NonInExport_entries_have_null_UserIdColumnName() =>
        Manifest.Entries
            .Where(e => e.Disposition is GdprDisposition.ExcludedFromExportAnonymisedInDeletion
                                      or GdprDisposition.ExcludedFromBoth)
            .Should().OnlyContain(e => e.UserIdColumnName == null);

    // ── M18-005 additions ─────────────────────────────────────────────────────

    [Fact]
    public void Manifest_contains_exactly_fourteen_tables_after_M18_005() =>
        Manifest.Entries.Should().HaveCount(14,
            because: "M18-005 adds deletion_reauth_tokens (InExportInDeletionHard)");

    [Fact]
    public void Deletion_reauth_token_disposition_is_InExportInDeletionHard()
    {
        var entry = Manifest.Entries.FirstOrDefault(e => e.TableName == "deletion_reauth_tokens");
        entry.Should().NotBeNull(because: "M18-005 adds deletion_reauth_tokens to the GDPR manifest");
        entry!.Disposition.Should().Be(GdprDisposition.InExportInDeletionHard);
    }

    [Fact]
    public void Deletion_reauth_token_has_user_id_column_name()
    {
        var entry = Manifest.Entries.FirstOrDefault(e => e.TableName == "deletion_reauth_tokens");
        entry.Should().NotBeNull();
        entry!.UserIdColumnName.Should().Be("UserId");
    }
}
