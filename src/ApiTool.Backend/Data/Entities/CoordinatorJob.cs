using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>A distributed execution job consisting of multiple shards.</summary>
public sealed class CoordinatorJob
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Organization that owns this job.</summary>
    public Guid OrgId { get; set; }

    /// <summary>
    /// User who created this job. NULL when the user has been anonymised (M18-006).
    /// </summary>
    [GdprIncluded(GdprDisposition.ExcludedFromExportAnonymisedInDeletion)]
    [GdprAnonymise(AnonymiseAs.SetNull)]
    public Guid? CreatedBy { get; set; }

    /// <summary>Opaque client-provided collection identifier being run.</summary>
    public string CollectionSha { get; set; } = string.Empty;

    /// <summary>Total number of shards this job is split into.</summary>
    public int ShardCount { get; set; }

    /// <summary>Current lifecycle state of the job.</summary>
    public CoordinatorJobStatus Status { get; set; } = CoordinatorJobStatus.Pending;

    /// <summary>When the job was created.</summary>
    public DateTime CreatedAt { get; set; }

    /// <summary>When the job was last updated.</summary>
    public DateTime UpdatedAt { get; set; }

    /// <summary>When the job completed (all shards done).</summary>
    public DateTime? CompletedAt { get; set; }

    /// <summary>Foreign key to the aggregated Result row written on completion.</summary>
    public Guid? AggregateResultId { get; set; }
}
