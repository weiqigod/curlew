namespace ApiTool.Backend.Organizations;

/// <summary>Request body for POST /api/v1/organizations/{id}/transfer.</summary>
/// <param name="NewOwnerId">User id (raw guid string) of the new owner.</param>
public sealed record TransferOwnershipRequest(string NewOwnerId);
