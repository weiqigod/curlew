namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Static tier-to-capability lookup for M14.
/// M16/M18 will replace this with a database-driven resolver.
/// Values documented in management/plans/M14-002-plan.md §Architectural Decision 7.
/// </summary>
public static class TierConfig
{
    private static readonly IReadOnlyList<string> FreeFeatures = [];
    private static readonly IReadOnlyList<string> ProfessionalFeatures =
        ["unlimited_requests", "collections", "environments"];
    private static readonly IReadOnlyList<string> TeamFeatures =
        ["unlimited_requests", "collections", "environments", "shared_collections", "sso"];
    private static readonly IReadOnlyList<string> EnterpriseFeatures =
        ["unlimited_requests", "collections", "environments", "shared_collections", "sso",
         "audit_log", "custom_roles"];

    /// <summary>
    /// Returns the <c>(features, request_limit)</c> tuple for the given <paramref name="tier"/> string.
    /// Falls back to free-tier defaults for unrecognised values.
    /// </summary>
    public static (IReadOnlyList<string> Features, int RequestLimit) For(string tier) =>
        tier switch
        {
            "professional" => (ProfessionalFeatures, 100_000),
            "team"         => (TeamFeatures, 1_000_000),
            "enterprise"   => (EnterpriseFeatures, int.MaxValue),
            _              => (FreeFeatures, 1_000),  // "free" and any unrecognised tier
        };
}
