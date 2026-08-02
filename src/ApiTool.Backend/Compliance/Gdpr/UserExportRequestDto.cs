namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Response DTO for both the POST (202) and GET (200) user data export endpoints.
/// Serialized as snake_case JSON. Nullable fields are omitted (WhenWritingNull).
/// </summary>
public sealed record UserExportRequestDto(
    string Id,
    string Status,
    string CreatedAt,
    string? ReadyAt,
    string? ExpiresAt,
    string? SignedUrl,
    string? FailureReason);
