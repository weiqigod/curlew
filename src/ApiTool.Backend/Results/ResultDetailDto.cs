namespace ApiTool.Backend.Results;

/// <summary>Detail response record for a test run (header + per-test rows).</summary>
public sealed record ResultDetailDto(
    string Id,
    string CollectionName,
    int PassCount,
    int FailCount,
    int SkippedCount,
    long DurationMs,
    DateTime RunAt,
    DateTime CreatedAt,
    string? TriggeredBy,
    string? GitSha,
    IReadOnlyList<ResultItemDto> Items);
