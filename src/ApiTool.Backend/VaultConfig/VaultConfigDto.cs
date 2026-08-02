namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Wire DTO returned by <see cref="VaultConfigService"/> get/upsert operations.
/// </summary>
/// <param name="Template">The raw YAML string (round-trippable, preserves comments).</param>
/// <param name="Version">Monotonic per-org version number.</param>
/// <param name="UpdatedAt">UTC timestamp of the last modification.</param>
/// <param name="UpdatedByEmail">Email of the user who last modified this config, when available.</param>
public sealed record VaultConfigDto(
    string Template,
    long Version,
    DateTime UpdatedAt,
    string? UpdatedByEmail);
