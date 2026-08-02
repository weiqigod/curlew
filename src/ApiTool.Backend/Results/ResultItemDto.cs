namespace ApiTool.Backend.Results;

/// <summary>Per-item nested record for the detail response.</summary>
public sealed record ResultItemDto(
    string Name,
    string Status,
    long DurationMs,
    string? Message);
