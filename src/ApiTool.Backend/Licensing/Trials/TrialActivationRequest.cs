namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Request body for <c>POST /api/v1/trials/{feature}</c>.
/// Mirrors <c>AuthRefreshRequest</c> so the CLI can pass the same cached credentials.
/// </summary>
public sealed record TrialActivationRequest(
    string? RefreshToken,
    Guid? DeviceId);
