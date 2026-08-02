namespace ApiTool.Backend.Results;

/// <summary>Per-item shape in the upload request body.</summary>
public sealed record UploadResultItemRequest(
    string? Name,
    string? Status,
    long? DurationMs,
    string? Message,
    string? Method = null,
    string? RequestUrl = null);
