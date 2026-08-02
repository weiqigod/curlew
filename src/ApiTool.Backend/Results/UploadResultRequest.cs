namespace ApiTool.Backend.Results;

/// <summary>
/// Request body for uploading a test run result.
/// All fields are nullable so that missing-field detection can point to the specific field.
/// </summary>
public sealed record UploadResultRequest(
    string? CollectionName,
    DateTime? RunAt,
    long? DurationMs,
    int? PassCount,
    int? FailCount,
    int? SkippedCount,
    string? TriggeredBy,
    string? GitSha,
    IReadOnlyList<UploadResultItemRequest>? Items);
