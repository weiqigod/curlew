using ApiTool.Backend.Rbac;

namespace ApiTool.Backend.Tests.Rbac;

/// <summary>Verifies the permission catalogue matches the specification matrix.</summary>
public sealed class PermissionsTests
{
    [Theory]
    [InlineData("members.invite")]
    [InlineData("results.upload")]
    [InlineData("dashboard.view")]
    [InlineData("members.remove")]
    [InlineData("members.view")]
    [InlineData("roles.change")]
    [InlineData("roles.view")]
    [InlineData("billing.manage")]
    [InlineData("billing.view")]
    [InlineData("seats.add")]
    [InlineData("seats.remove")]
    [InlineData("org.settings.manage")]
    [InlineData("org.settings.view")]
    [InlineData("org.delete")]
    [InlineData("org.transfer")]
    [InlineData("tokens.create")]
    [InlineData("tokens.revoke")]
    [InlineData("tokens.view")]
    [InlineData("results.view")]
    [InlineData("results.delete")]
    [InlineData("vault_config.manage")]
    [InlineData("vault_config.view")]
    [InlineData("dashboard.export")]
    [InlineData("coordinator.worker")]
    public void All_contains_spec_keys(string key) =>
        Permissions.All.Should().Contain(key);

    [Theory]
    [InlineData("audit_log.view")]
    [InlineData("audit_log.export")]
    public void All_contains_audit_log_keys(string key) =>
        Permissions.All.Should().Contain(key);

    [Fact]
    public void All_has_26_entries_matching_spec_matrix() =>
        Permissions.All.Should().HaveCount(26);

    [Fact]
    public void BuiltInAdmin_contains_audit_log_view_and_export()
    {
        Permissions.BuiltInAdmin.Should().Contain(Permissions.AuditLogView);
        Permissions.BuiltInAdmin.Should().Contain(Permissions.AuditLogExport);
    }

    [Fact]
    public void BuiltInMember_does_not_contain_audit_log_permissions()
    {
        Permissions.BuiltInMember.Should().NotContain(Permissions.AuditLogView);
        Permissions.BuiltInMember.Should().NotContain(Permissions.AuditLogExport);
    }

    [Fact]
    public void BuiltInOwner_contains_both_audit_log_permissions()
    {
        Permissions.BuiltInOwner.Should().Contain(Permissions.AuditLogView);
        Permissions.BuiltInOwner.Should().Contain(Permissions.AuditLogExport);
    }

    [Fact]
    public void CoordinatorWorker_is_in_all_built_in_sets()
    {
        Permissions.All.Should().Contain("coordinator.worker");
        Permissions.BuiltInOwner.Should().Contain("coordinator.worker");
        Permissions.BuiltInAdmin.Should().Contain("coordinator.worker");
        Permissions.BuiltInMember.Should().Contain("coordinator.worker");
    }

    [Fact]
    public void Owner_set_is_superset_of_admin_set() =>
        Permissions.BuiltInAdmin.All(k => Permissions.BuiltInOwner.Contains(k)).Should().BeTrue();

    [Fact]
    public void Admin_has_members_invite_but_member_does_not()
    {
        Permissions.BuiltInAdmin.Should().Contain(Permissions.MembersInvite);
        Permissions.BuiltInMember.Should().NotContain(Permissions.MembersInvite);
    }

    [Fact]
    public void Owner_only_permissions_absent_from_admin()
    {
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.RolesChange);
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.BillingManage);
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.OrgDelete);
    }

    [Fact]
    public void Member_has_view_only_permissions()
    {
        Permissions.BuiltInMember.Should().Contain(Permissions.ResultsView);
        Permissions.BuiltInMember.Should().Contain(Permissions.DashboardView);
        Permissions.BuiltInMember.Should().NotContain(Permissions.ResultsDelete);
    }

    [Fact]
    public void BuiltInOwner_is_superset_of_all_other_sets()
    {
        Permissions.BuiltInMember.All(k => Permissions.BuiltInOwner.Contains(k)).Should().BeTrue();
    }

    [Fact]
    public void All_sets_are_subsets_of_All()
    {
        Permissions.BuiltInOwner.All(k => Permissions.All.Contains(k)).Should().BeTrue();
        Permissions.BuiltInAdmin.All(k => Permissions.All.Contains(k)).Should().BeTrue();
        Permissions.BuiltInMember.All(k => Permissions.All.Contains(k)).Should().BeTrue();
    }

    [Fact]
    public void Member_does_not_have_billing_view_per_spec_matrix()
    {
        // Spec matrix line 7095: billing.view is ✗ for Member.
        Permissions.BuiltInMember.Should().NotContain(Permissions.BillingView);
    }

    [Fact]
    public void Admin_does_not_have_seats_add_per_spec_matrix()
    {
        // Spec matrix line 7096: seats.add is ✓ Owner, ✗ Admin, ✗ Member.
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.SeatsAdd);
        Permissions.BuiltInMember.Should().NotContain(Permissions.SeatsAdd);
    }

    [Fact]
    public void Admin_does_not_have_seats_remove_per_spec_matrix()
    {
        // Spec matrix: seats.remove is ✓ Owner, ✗ Admin, ✗ Member.
        Permissions.BuiltInAdmin.Should().NotContain(Permissions.SeatsRemove);
        Permissions.BuiltInMember.Should().NotContain(Permissions.SeatsRemove);
    }
}
