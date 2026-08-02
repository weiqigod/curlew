using System.Reflection;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Reflects over the EF entity assembly and emits a <see cref="GdprBundleManifest"/>
/// describing every table's GDPR disposition and the annotated user-attribution columns.
/// Consumed by M18-004 (export builder) and M18-006 (anonymiser).
/// </summary>
public static class GdprAttributeScanner
{
    // Binds to the actual entity namespace via reflection so a future rename of the User
    // entity's namespace forces the scanner to compile against the new layout, instead of
    // silently dropping every entity from the manifest.
    private static readonly string EntityNamespace = typeof(User).Namespace
        ?? throw new InvalidOperationException("User entity must declare a namespace.");

    /// <summary>
    /// Lazily-built manifest, evaluated once per process from <c>typeof(AppDbContext).Assembly</c>.
    /// </summary>
    public static GdprBundleManifest Manifest { get; } = Scan(typeof(AppDbContext).Assembly);

    /// <summary>
    /// Scans <paramref name="assembly"/> for types in the entity namespace
    /// (<c>typeof(User).Namespace</c>) that carry GDPR attributes and returns a
    /// deterministic, alphabetically-ordered <see cref="GdprBundleManifest"/>.
    /// </summary>
    public static GdprBundleManifest Scan(Assembly assembly)
    {
        ArgumentNullException.ThrowIfNull(assembly);

        var entries = new List<GdprManifestEntry>();

        var entityTypes = assembly.GetTypes()
            .Where(t => t.IsClass && !t.IsAbstract && t.Namespace == EntityNamespace)
            .OrderBy(t => t.Name, StringComparer.Ordinal);

        foreach (var type in entityTypes)
        {
            var classAttr = type.GetCustomAttribute<GdprTableAttribute>();

            // Explicit opt-out — not in the v4-4 inventory scope.
            if (classAttr?.Kind == GdprTableKind.NotUserAttributable)
                continue;

            // Class-level ExcludedFromBoth: enumerated in v4-4 but has no user-attribution column.
            if (classAttr?.Kind == GdprTableKind.ExcludedFromBoth)
            {
                entries.Add(new GdprManifestEntry(
                    TableName: TableNameOf(type),
                    EntityType: type,
                    Disposition: GdprDisposition.ExcludedFromBoth,
                    Columns: []));
                continue;
            }

            // Collect property-level annotations.
            var columns = type
                .GetProperties(BindingFlags.Public | BindingFlags.Instance)
                .Select(p => new
                {
                    Property = p,
                    Included = p.GetCustomAttribute<GdprIncludedAttribute>(),
                    Anonymise = p.GetCustomAttribute<GdprAnonymiseAttribute>(),
                })
                .Where(x => x.Included is not null || x.Anonymise is not null)
                // Columns tagged with only [GdprAnonymise] (no [GdprIncluded]) inherit
                // InExportInDeletionAnonymise — the disposition of OrganizationAuditLogEntry.ActorEmail
                // is the canonical case. Pinned by GdprAttributeScannerTests
                // .Audit_log_actor_email_column_disposition_is_InExportInDeletionAnonymise.
                .Select(x => new GdprColumnEntry(
                    PropertyName: x.Property.Name,
                    Disposition: x.Included?.Disposition ?? GdprDisposition.InExportInDeletionAnonymise,
                    AnonymiseAs: x.Anonymise?.Kind))
                .OrderBy(c => c.PropertyName, StringComparer.Ordinal)
                .ToList();

            // No annotations and no class attribute — skip; not in the inventory.
            if (columns.Count == 0)
                continue;

            // Table disposition is the minimum (highest-priority) disposition across its columns.
            // Enum integer values: InExportInDeletionHard(0) < InExportInDeletionAnonymise(1)
            //                    < ExcludedFromExportAnonymisedInDeletion(2) < ExcludedFromBoth(3)
            // Lower value = stronger inclusion right, so we take the min.
            var tableDisposition = (GdprDisposition)columns.Min(c => (int)c.Disposition);

            // M18-004: find the property tagged [GdprUserAttribution] — exists only on InExport tables.
            var userIdColumn = type
                .GetProperties(BindingFlags.Public | BindingFlags.Instance)
                .FirstOrDefault(p => p.GetCustomAttribute<GdprUserAttributionAttribute>() is not null)
                ?.Name;

            entries.Add(new GdprManifestEntry(
                TableName: TableNameOf(type),
                EntityType: type,
                Disposition: tableDisposition,
                Columns: columns,
                UserIdColumnName: userIdColumn));
        }

        return new GdprBundleManifest(entries);
    }

    // Hardcoded snake-case map matching AppDbContext.OnModelCreating ToTable(...) calls.
    // Reading EF model metadata would require a live DbContext; this mirror is acceptable
    // because Table_names_match_AppDbContext_ToTable_calls (in tests) compares every entry
    // against the live EF model and fails on any drift.
    private static string TableNameOf(Type t) => t.Name switch
    {
        nameof(User) => "users",
        nameof(RefreshToken) => "refresh_tokens",
        nameof(OrganizationMember) => "organization_members",
        nameof(EmailVerificationToken) => "email_verification_tokens",
        nameof(PasswordResetToken) => "password_reset_tokens",
        nameof(DeletionReauthToken) => "deletion_reauth_tokens",
        nameof(NotificationRule) => "notification_rules",
        nameof(CustomRole) => "organization_custom_roles",
        nameof(Schedule) => "schedules",
        nameof(CoordinatorJob) => "coordinator_jobs",
        nameof(TeamVault) => "team_vaults",
        nameof(OrganizationAuditLogEntry) => "organization_audit_log",
        nameof(GithubInstallation) => "github_installations",
        nameof(GitLabInstallation) => "gitlab_installations",
        _ => t.Name.ToLowerInvariant(),
    };
}
