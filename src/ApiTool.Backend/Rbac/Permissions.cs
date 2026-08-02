namespace ApiTool.Backend.Rbac;

/// <summary>Canonical catalogue of organization permission keys.</summary>
/// <remarks>
/// Permission keys are lowercase dot-separated strings on the wire (e.g. <c>results.upload</c>).
/// The three built-in sets mirror the spec's permission matrix.
/// </remarks>
public static class Permissions
{
    /// <summary>Invite new members to the organization.</summary>
    public const string MembersInvite = "members.invite";

    /// <summary>Remove members from the organization.</summary>
    public const string MembersRemove = "members.remove";

    /// <summary>View the member list.</summary>
    public const string MembersView = "members.view";

    /// <summary>Assign custom roles to members.</summary>
    public const string RolesChange = "roles.change";

    /// <summary>View the roles catalogue.</summary>
    public const string RolesView = "roles.view";

    /// <summary>Manage billing subscriptions.</summary>
    public const string BillingManage = "billing.manage";

    /// <summary>View billing and invoice information.</summary>
    public const string BillingView = "billing.view";

    /// <summary>Add seats to the subscription.</summary>
    public const string SeatsAdd = "seats.add";

    /// <summary>Remove seats from the subscription.</summary>
    public const string SeatsRemove = "seats.remove";

    /// <summary>Manage organization-level settings.</summary>
    public const string OrgSettingsManage = "org.settings.manage";

    /// <summary>View organization-level settings.</summary>
    public const string OrgSettingsView = "org.settings.view";

    /// <summary>Delete the organization.</summary>
    public const string OrgDelete = "org.delete";

    /// <summary>Transfer organization ownership.</summary>
    public const string OrgTransfer = "org.transfer";

    /// <summary>Create API tokens.</summary>
    public const string TokensCreate = "tokens.create";

    /// <summary>Revoke API tokens.</summary>
    public const string TokensRevoke = "tokens.revoke";

    /// <summary>View API tokens.</summary>
    public const string TokensView = "tokens.view";

    /// <summary>View test results.</summary>
    public const string ResultsView = "results.view";

    /// <summary>Upload test results.</summary>
    public const string ResultsUpload = "results.upload";

    /// <summary>Delete test results.</summary>
    public const string ResultsDelete = "results.delete";

    /// <summary>Manage vault configuration (secrets).</summary>
    public const string VaultConfigManage = "vault_config.manage";

    /// <summary>View vault configuration.</summary>
    public const string VaultConfigView = "vault_config.view";

    /// <summary>View dashboards.</summary>
    public const string DashboardView = "dashboard.view";

    /// <summary>Export dashboard data.</summary>
    public const string DashboardExport = "dashboard.export";

    /// <summary>Claim coordinator shards and submit worker results.</summary>
    public const string CoordinatorWorker = "coordinator.worker";

    /// <summary>View the organization audit log.</summary>
    public const string AuditLogView = "audit_log.view";

    /// <summary>Export the organization audit log (bulk JSONL/CSV).</summary>
    public const string AuditLogExport = "audit_log.export";

    /// <summary>Immutable set of all valid permission keys.</summary>
    public static readonly IReadOnlySet<string> All;

    /// <summary>Permissions available to the built-in owner role (all permissions).</summary>
    public static readonly IReadOnlySet<string> BuiltInOwner;

    /// <summary>Permissions available to the built-in admin role.</summary>
    public static readonly IReadOnlySet<string> BuiltInAdmin;

    /// <summary>Permissions available to the built-in member role.</summary>
    public static readonly IReadOnlySet<string> BuiltInMember;

    static Permissions()
    {
        All = new HashSet<string>(StringComparer.Ordinal)
        {
            MembersInvite, MembersRemove, MembersView,
            RolesChange, RolesView,
            BillingManage, BillingView,
            SeatsAdd, SeatsRemove,
            OrgSettingsManage, OrgSettingsView,
            OrgDelete, OrgTransfer,
            TokensCreate, TokensRevoke, TokensView,
            ResultsView, ResultsUpload, ResultsDelete,
            VaultConfigManage, VaultConfigView,
            DashboardView, DashboardExport,
            CoordinatorWorker,
            AuditLogView, AuditLogExport,
        };

        // Member: standard contributor access — can view and upload results, view dashboard, and view org data.
        BuiltInMember = new HashSet<string>(StringComparer.Ordinal)
        {
            MembersView,
            RolesView,
            OrgSettingsView,
            TokensView,
            ResultsView, ResultsUpload,
            VaultConfigView,
            DashboardView, DashboardExport,
            CoordinatorWorker,
        };

        // Admin: everything a member can do, plus member management and tokens.
        BuiltInAdmin = new HashSet<string>(StringComparer.Ordinal)
        {
            MembersInvite, MembersRemove, MembersView,
            RolesView,
            BillingView,
            OrgSettingsManage, OrgSettingsView,
            TokensCreate, TokensRevoke, TokensView,
            ResultsView, ResultsUpload, ResultsDelete,
            VaultConfigManage, VaultConfigView,
            DashboardView, DashboardExport,
            CoordinatorWorker,
            AuditLogView, AuditLogExport,
        };

        // Owner: all permissions.
        BuiltInOwner = All;
    }
}
