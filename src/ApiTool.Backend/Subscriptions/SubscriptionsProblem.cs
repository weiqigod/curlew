// RFC 7807 problem-details responses for subscription/billing failures.
// Spec refs: docs/SPECIFICATION.md:7322-7345.
using Microsoft.AspNetCore.Mvc;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Builds RFC 7807 ProblemDetails responses for subscription and billing failures.
/// Mirrors the RefreshProblem shape from <c>Auth/Refresh/RefreshProblem.cs</c>.
/// </summary>
internal static class SubscriptionsProblem
{
    private const string BaseType = "https://api.apitool.dev/errors";

    /// <summary>HTTP 502 — Stripe returned a 5xx or the request failed to reach Stripe.</summary>
    public static IResult StripeUnavailable(HttpContext http, string? requestId, string? idempotencyKey) =>
        Build(http, 502,
            type:   $"{BaseType}/stripe-unavailable",
            title:  "Stripe API is unavailable",
            detail: "The Stripe API returned a 5xx response or is unreachable. Retry with the same Idempotency-Key.",
            code:   "STRIPE_UNAVAILABLE",
            extensions: new Dictionary<string, object?>
            {
                ["stripe_request_id"] = requestId,
                ["idempotency_key"]   = idempotencyKey,
            });

    /// <summary>HTTP 409 — org has no Stripe customer on file; checkout must occur first.</summary>
    public static IResult NoBillingSetup(HttpContext http) =>
        Build(http, 409,
            type:   $"{BaseType}/no-billing-setup",
            title:  "No billing setup",
            detail: "Organization has no Stripe customer on file. Complete checkout first.",
            code:   "no_billing_setup");

    /// <summary>HTTP 409 — org has no active subscription; preview-proration is not possible.</summary>
    public static IResult NoActiveSubscription(HttpContext http) =>
        Build(http, 409,
            type:   $"{BaseType}/no-active-subscription",
            title:  "No active subscription",
            detail: "Organization has no active Stripe subscription. Complete checkout first.",
            code:   "no_active_subscription");

    /// <summary>HTTP 403 — caller lacks admin or owner role.</summary>
    public static IResult PermissionDenied(HttpContext http, string detail) =>
        Build(http, 403,
            type:   $"{BaseType}/permission-denied",
            title:  "Permission denied",
            detail: detail,
            code:   "permission_denied");

    /// <summary>HTTP 503 — Stripe returned HTTP 429 rate-limit.</summary>
    public static IResult StripeRateLimited(HttpContext http, string? idempotencyKey) =>
        Build(http, 503,
            type:   $"{BaseType}/stripe-rate-limited",
            title:  "Stripe rate-limited",
            detail: "Stripe returned HTTP 429. Retry with the same Idempotency-Key.",
            code:   "stripe_rate_limited",
            extensions: new Dictionary<string, object?>
            {
                ["idempotency_key"] = idempotencyKey,
            });

    private static IResult Build(
        HttpContext http,
        int status,
        string type,
        string title,
        string detail,
        string code,
        IReadOnlyDictionary<string, object?>? extensions = null)
    {
        var requestId = http.TraceIdentifier;
        http.Response.Headers["X-Request-Id"] = requestId;

        var problem = new ProblemDetails
        {
            Type   = type,
            Title  = title,
            Status = status,
            Detail = detail,
            Extensions =
            {
                ["code"]       = code,
                ["request_id"] = requestId,
            },
        };
        if (extensions is not null)
            foreach (var (k, v) in extensions)
                if (v is not null) problem.Extensions[k] = v;

        return HttpResults.Json(problem, contentType: "application/problem+json", statusCode: status);
    }
}
