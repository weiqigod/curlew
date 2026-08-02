namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.AspNetCore.Builder;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Routing;
using Microsoft.EntityFrameworkCore;

/// <summary>
/// Testing/Development-only probe endpoint that exercises the Tier-Gate plumbing
/// end-to-end before the real vault-config / schedule / dashboard endpoints land.
/// Mirrors the gating pattern of <c>InternalSeedM14Endpoint</c>.
/// </summary>
internal static class InternalTierGateProbeEndpoint
{
    /// <summary>
    /// Maps the tier-gate probe routes. Only registers when not in Production
    /// so this surface never appears in the production binary.
    /// </summary>
    public static void MapInternalTierGateProbeEndpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (env.IsProduction()) return;

        app.MapPost(
            "/internal/test/tier-gate-probe/{orgId}/vault-config",
            async (Guid orgId, ITierGate gate, AppDbContext db, HttpContext http, CancellationToken ct) =>
            {
                var result = await VaultConfigTierGate.EnsureTeamOrAboveAsync(gate, orgId, ct);

                if (result == VaultConfigError.None)
                    return Results.Ok();

                if (result == VaultConfigError.OrgNotFound)
                    return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

                // Resolve the org's current tier for the 402 problem detail.
                var current = await db.Subscriptions
                    .Where(s => s.OrgId == orgId)
                    .OrderByDescending(s => s.UpdatedAt)
                    .Select(s => (SubscriptionTier?)s.Tier)
                    .FirstOrDefaultAsync(ct) ?? SubscriptionTier.Free;

                return TierGateProblemFactory.AuthenticatedTierIneligible(
                    http, current, VaultConfigTierGate.RequiredMinimum, "vault_config");
            });

        app.MapGet(
            "/internal/test/tier-gate-probe/{orgId}/sso-login",
            async (Guid orgId, ITierGate gate, CancellationToken ct) =>
            {
                var result = await gate.EnsureAsync(orgId, SubscriptionTier.Enterprise, ct);

                if (result == TierGateResult.Allowed)
                    return Results.Ok();

                return TierGateProblemFactory.PublicFlowNotFound();
            });
    }
}
