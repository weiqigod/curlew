namespace ApiTool.Backend.Internal.TierGates;

using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;

/// <summary>
/// Builds RFC 7807 responses for tier-gate denials (v4.3). Mirrors
/// <c>SubscriptionsProblem</c> shape and conventions.
/// </summary>
public static class TierGateProblemFactory
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>
    /// Authenticated org-scoped endpoint denial: 402 Payment Required with the
    /// org's current tier and the required minimum embedded.
    /// </summary>
    public static IResult AuthenticatedTierIneligible(
        HttpContext http,
        SubscriptionTier currentTier,
        SubscriptionTier requiredMinimumTier,
        string featureCode)
    {
        ArgumentException.ThrowIfNullOrEmpty(featureCode);

        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = $"{BaseType}/tier-ineligible",
            Title  = "Tier ineligible",
            Status = StatusCodes.Status402PaymentRequired,
            Detail = $"This feature requires the {requiredMinimumTier.ToString().ToLowerInvariant()} tier or above.",
            Extensions =
            {
                ["code"]          = $"{featureCode}_tier_ineligible",
                ["request_id"]    = requestId,
                ["current_tier"]  = currentTier.ToString().ToLowerInvariant(),
                ["required_tier"] = requiredMinimumTier.ToString().ToLowerInvariant(),
            },
        };

        return Results.Json(
            problem,
            contentType: "application/problem+json",
            statusCode: StatusCodes.Status402PaymentRequired);
    }

    /// <summary>
    /// Authenticated org-scoped endpoint where the org does not exist:
    /// 404 Not Found, plain ProblemDetails, no cache-suppression header (still
    /// behind auth, no info-leak class).
    /// </summary>
    public static IResult AuthenticatedOrgNotFound(HttpContext http)
    {
        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = $"{BaseType}/organization-not-found",
            Title  = "Organization not found",
            Status = StatusCodes.Status404NotFound,
            Extensions =
            {
                ["code"]       = "organization_not_found",
                ["request_id"] = requestId,
            },
        };

        return Results.Json(
            problem,
            contentType: "application/problem+json",
            statusCode: StatusCodes.Status404NotFound);
    }

    /// <summary>
    /// Resolves the org's current tier from the database and builds an
    /// <see cref="AuthenticatedTierIneligible"/> response. Convenience overload for callers
    /// that don't already have the tier at hand; performs one extra DB query.
    /// </summary>
    public static async Task<IResult> AuthenticatedTierIneligibleAsync(
        HttpContext http,
        AppDbContext db,
        Guid orgId,
        SubscriptionTier requiredMinimumTier,
        string featureCode,
        CancellationToken ct)
    {
        var currentTier = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .OrderByDescending(s => s.UpdatedAt)
            .Select(s => (SubscriptionTier?)s.Tier)
            .FirstOrDefaultAsync(ct) ?? SubscriptionTier.Free;

        return AuthenticatedTierIneligible(http, currentTier, requiredMinimumTier, featureCode);
    }

    /// <summary>
    /// Public-flow endpoint denial (mirrors <c>SsoNotFoundResult</c>): 404 with
    /// <c>Cache-Control: no-store</c> and no body — indistinguishable from a
    /// non-existent org so external probes cannot infer tier metadata.
    /// </summary>
    public static IResult PublicFlowNotFound() => new PublicFlowNotFoundResult();

    private sealed class PublicFlowNotFoundResult : IResult
    {
        public Task ExecuteAsync(HttpContext httpContext)
        {
            httpContext.Response.StatusCode = StatusCodes.Status404NotFound;
            httpContext.Response.Headers.CacheControl = "no-store";
            httpContext.Response.ContentLength = 0;
            return Task.CompletedTask;
        }
    }
}
