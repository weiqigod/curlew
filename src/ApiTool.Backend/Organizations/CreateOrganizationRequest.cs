namespace ApiTool.Backend.Organizations;

/// <summary>Request body for creating a new organization.</summary>
/// <param name="Name">Human-readable display name.</param>
/// <param name="Slug">URL-safe lowercase slug; must be globally unique.</param>
public sealed record CreateOrganizationRequest(string Name, string Slug);
