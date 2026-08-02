namespace ApiTool.Backend.Organizations;

/// <summary>Response shape for an organization resource.</summary>
/// <param name="Id">Wire-format organization id (<c>org_&lt;hex&gt;</c>).</param>
/// <param name="Name">Display name.</param>
/// <param name="Slug">URL-safe lowercase slug.</param>
/// <param name="Role">The requesting user's role in this organization.</param>
/// <param name="Tier">Current subscription tier in lowercase wire format.</param>
/// <param name="SeatCount">Current number of members (computed, not stored).</param>
/// <param name="SeatLimit">Maximum allowed seats for the current tier.</param>
/// <param name="Status">Lifecycle status string.</param>
/// <param name="CreatedAt">UTC creation timestamp.</param>
public sealed record OrganizationDto(
    string Id,
    string Name,
	string Slug,
	string Role,
	string Tier,
	int SeatCount,
    int SeatLimit,
    string Status,
    DateTime CreatedAt);
