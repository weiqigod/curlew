namespace ApiTool.Backend.Organizations;

/// <summary>
/// Minimal DTO carrying the identity of an organization that blocks a user's
/// account deletion because the user is its sole Owner and other members remain.
/// Returned by <see cref="LastAdminProtectionService.ClassifyOwnedOrgsAsync"/>.
/// </summary>
public sealed record BlockingOrg(string Slug, string Name);
