using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>
/// Backend-stored shared vault configuration template (one row per organization, Team tier).
/// Refs docs/SPECIFICATION.md:10945-10962 (schema), :5654-5715 (layer 4 design).
/// </summary>
public sealed class TeamVault
{
    /// <summary>Owning organization id. Primary key — one row per org.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Round-trippable YAML source. Preserves comments + ordering for the editor.</summary>
    public string TemplateYaml { get; set; } = string.Empty;

    /// <summary>Parsed form stored as JSON; used by the validator and downstream consumers.</summary>
    public string TemplateJson { get; set; } = "{}";

    /// <summary>
    /// AES-256-GCM envelope blob of the parsed template JSON (M18-009, v4-12).
    /// Null until the backfill host has migrated this row; coexists with <see cref="TemplateJson"/>
    /// during the expand-migrate-contract window (Migration 1 adds columns; Migration 2 drops plaintext).
    /// </summary>
    public byte[]? TemplateJsonCiphertext { get; set; }

    /// <summary>
    /// Key-encryption-key identifier that wrapped the per-row DEK stored in <see cref="TemplateJsonCiphertext"/>.
    /// Null until the backfill host has migrated this row.
    /// </summary>
    public string? TemplateJsonKid { get; set; }

    /// <summary>Monotonic per-org version; surfaces in the GET response ETag.</summary>
    public long Version { get; set; }

    /// <summary>Insert moment (UTC).</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>
    /// User id of the actor who first wrote this row. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? CreatedBy { get; set; }

    /// <summary>Last mutation moment (UTC).</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>
    /// User id of the actor who last wrote this row. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? UpdatedBy { get; set; }
}
