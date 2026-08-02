using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>A cron-scheduled test run configuration scoped to an organization.</summary>
public sealed class Schedule
{
    /// <summary>Primary key (serialized as <c>sched_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization foreign key.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Human-readable schedule name (unique per org).</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>Standard 5-field cron expression (e.g. "0 2 * * *").</summary>
    public string CronExpression { get; set; } = string.Empty;

    /// <summary>IANA TZ identifier the cron expression is evaluated in. Defaults to "UTC".</summary>
    public string Timezone { get; set; } = "UTC";

    /// <summary>Reference to the collection file to run (e.g. "smoke.yaml").</summary>
    public string CollectionRef { get; set; } = string.Empty;

    /// <summary>Whether this schedule is currently active.</summary>
    public bool Enabled { get; set; } = true;

    /// <summary>Computed next UTC time the schedule should fire.</summary>
    public DateTime? NextRunAt { get; set; }

    /// <summary>UTC time of the last completed/queued run, or null if never fired.</summary>
    public DateTime? LastRunAt { get; set; }

    /// <summary>
    /// Id of the user who created this schedule. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? CreatedBy { get; set; }

    /// <summary>UTC creation timestamp.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>UTC last-updated timestamp.</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>
    /// AES-256-GCM envelope blob of the env_vars dictionary serialised as UTF-8 JSON (M18-009, v4-12).
    /// Null when the schedule has no env_vars. New rows store ciphertext from the start;
    /// there is no plaintext predecessor for this column.
    /// </summary>
    public byte[]? EnvVarsCiphertext { get; set; }

    /// <summary>
    /// Key-encryption-key identifier that wrapped the per-row DEK stored in <see cref="EnvVarsCiphertext"/>.
    /// Null when <see cref="EnvVarsCiphertext"/> is null.
    /// </summary>
    public string? EnvVarsKid { get; set; }
}
