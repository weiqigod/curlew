using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>An organization that groups users and owns API test resources.</summary>
[GdprTable(GdprTableKind.NotUserAttributable)]
public sealed class Organization
{
    /// <summary>Primary key (UUID stored in SQL, serialized as <c>org_&lt;hex&gt;</c> on the wire).</summary>
    public Guid Id { get; set; }

    /// <summary>Human-readable display name.</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>URL-safe lowercase slug — must be globally unique.</summary>
    public string Slug { get; set; } = string.Empty;

    /// <summary>Foreign key to the user who created (and owns) this organization.</summary>
    public Guid OwnerId { get; set; }

    /// <summary>Current lifecycle status.</summary>
    public OrgStatus Status { get; set; } = OrgStatus.Active;

    /// <summary>UTC timestamp when the row was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC timestamp of the last update.</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>JSON blob for arbitrary organization settings. Defaults to an empty JSON object.</summary>
    public string SettingsJson { get; set; } = "{}";

    /// <summary>
    /// Number of days to retain audit log entries for this organization (M18-002).
    /// Defaults to 365. Non-Enterprise orgs are capped at 365 at the write site.
    /// </summary>
    public int AuditLogRetentionDays { get; set; } = 365;
}
