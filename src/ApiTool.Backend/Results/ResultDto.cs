namespace ApiTool.Backend.Results;

/// <summary>List-item response record for a test run header.</summary>
public sealed record ResultDto(
    string Id,
    string CollectionName,
    int PassCount,
    int FailCount,
    int SkippedCount,
    long DurationMs,
    DateTime RunAt,
    DateTime CreatedAt,
    string? TriggeredBy,
    string? GitSha);
