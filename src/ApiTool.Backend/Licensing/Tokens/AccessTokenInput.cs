namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Input data for minting an Access token.
/// Refs docs/SPECIFICATION.md:7878-7891 (Access token 9-claim shape).
/// </summary>
public sealed record AccessTokenInput(
    Guid UserId,
    string Tier,
    Guid? OrgId,
    Guid DeviceId);
