using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Data.Entities;

/// <summary>Aggregate header row for a single uploaded test run.</summary>
[GdprTable(GdprTableKind.NotUserAttributable)]
public sealed class Result
{
    /// <summary>Primary key (serialized as <c>res_&lt;hex&gt;</c>).</summary>
    public Guid Id { get; set; }

    /// <summary>Owning organization foreign key.</summary>
    public Guid OrgId { get; set; }

    /// <summary>Id of the user who uploaded this run.</summary>
    public Guid UploadedBy { get; set; }

    /// <summary>Collection/file name reported by the CLI.</summary>
    public string CollectionName { get; set; } = string.Empty;

    /// <summary>UTC timestamp the run started (client-reported).</summary>
    public DateTime RunAt { get; set; }

    /// <summary>Total duration in milliseconds (client-reported).</summary>
    public long DurationMs { get; set; }

    /// <summary>Number of passing tests.</summary>
    public int PassCount { get; set; }

    /// <summary>Number of failing tests.</summary>
    public int FailCount { get; set; }

    /// <summary>Number of skipped tests.</summary>
    public int SkippedCount { get; set; }

    /// <summary>Free-form triggered-by label (e.g. "cli", "scheduled", "ci").</summary>
    public string? TriggeredBy { get; set; }

    /// <summary>Optional git commit SHA associated with the run.</summary>
    public string? GitSha { get; set; }

    /// <summary>Server-side ingestion timestamp (UTC).</summary>
    public DateTime CreatedAt { get; set; }
}
