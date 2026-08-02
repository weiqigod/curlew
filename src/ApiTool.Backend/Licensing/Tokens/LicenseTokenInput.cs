namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Input data for minting a License JWT.
/// Refs docs/SPECIFICATION.md:7854-7876 (License JWT claim set).
/// </summary>
public sealed record LicenseTokenInput(
    Guid UserId,
    string Email,
    string Tier,
    Guid? OrgId,
    string? OrgRole,
    Guid DeviceId,
    IReadOnlyList<string> Features,
    int RequestLimit);
