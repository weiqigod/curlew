namespace ApiTool.Backend.Data.Entities;

/// <summary>Per-test row for a <see cref="Result"/>.</summary>
public sealed class ResultItem
{
    /// <summary>Primary key.</summary>
    public Guid Id { get; set; }

    /// <summary>Owning result foreign key.</summary>
    public Guid ResultId { get; set; }

    /// <summary>Zero-based ordering index for stable display.</summary>
    public int Ordinal { get; set; }

    /// <summary>Test case name.</summary>
    public string Name { get; set; } = string.Empty;

    /// <summary>Outcome.</summary>
    public ResultStatus Status { get; set; }

    /// <summary>Per-test duration in milliseconds.</summary>
    public long DurationMs { get; set; }

    /// <summary>Error/assertion message when <see cref="Status"/> is not <c>Passed</c>.</summary>
    public string? Message { get; set; }

    /// <summary>HTTP method (uppercase). Null for non-HTTP items or legacy rows.</summary>
    public string? Method { get; set; }

    /// <summary>Original request URL (full or path-only). Source for path-template extraction.</summary>
    public string? RequestUrl { get; set; }

    /// <summary>Normalised path template (e.g. <c>/users/{id}</c>). Null when not derivable.</summary>
    public string? PathTemplate { get; set; }
}
