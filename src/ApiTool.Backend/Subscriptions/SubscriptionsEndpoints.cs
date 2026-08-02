using ApiTool.Backend.Auth;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Options;
using ApiTool.Backend;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Subscriptions;

/// <summary>Registers the <c>/api/v1/subscriptions</c> endpoint group.</summary>
public static class SubscriptionsEndpoints
{
    /// <summary>Maps all subscription endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapSubscriptionsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/subscriptions")
            .RequireAuthorization()
            .WithTags("Subscriptions");

        group.MapPost("checkout", CreateCheckout)
            .WithName("CreateCheckout")
            .RequireVerifiedEmail()   // M16-003: gates on email_verified = true
            .Produces<CheckoutResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .ProducesProblem(StatusCodes.Status403Forbidden);

        group.MapGet("", GetSubscription)
            .WithName("GetSubscription")
            .Produces<GetSubscriptionResponse>()
            .Produces(StatusCodes.Status401Unauthorized);

        group.MapPatch("{id}", UpdateSubscription)
            .WithName("UpdateSubscription")
            .Produces<UpdateSubscriptionResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapDelete("{id}", CancelSubscription)
            .WithName("CancelSubscription")
            .Produces<CancelSubscriptionResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("{id}/reactivate", ReactivateSubscription)
            .WithName("ReactivateSubscription")
            .Produces<ReactivateSubscriptionResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapPost("portal", CreatePortal)
            .WithName("CreatePortal")
            .Produces<PortalResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        group.MapPost("billing-portal", CreateBillingPortal)
            .WithName("CreateBillingPortal")
            .Produces<BillingPortalResponse>()
            .Produces<ProblemDetails>(StatusCodes.Status403Forbidden)
            .Produces<ProblemDetails>(StatusCodes.Status409Conflict)
            .Produces<ProblemDetails>(StatusCodes.Status502BadGateway)
            .Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable);

        group.MapPost("preview-proration", PreviewProration)
            .WithName("PreviewProration")
            .Produces<PreviewProrationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ProblemDetails>(StatusCodes.Status403Forbidden)
            .Produces<ProblemDetails>(StatusCodes.Status409Conflict)
            .Produces<ProblemDetails>(StatusCodes.Status502BadGateway)
            .Produces<ProblemDetails>(StatusCodes.Status503ServiceUnavailable);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> CreateCheckout(
        [FromBody] CheckoutRequest body,
        HttpContext context,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        // Read or synthesise an idempotency key (behaviour #1/#2).
        var idempotencyKey = context.Request.Headers["Idempotency-Key"].FirstOrDefault()
                             ?? Guid.NewGuid().ToString();

        // price_id path (behaviour #6: org resolved from bearer token, body org_id ignored).
        if (!string.IsNullOrEmpty(body.PriceId))
        {
            var (checkoutUrl, sessionId, err, msg) = await svc.CreateCheckoutByPriceAsync(
                userId.Value, body.PriceId, body.SuccessUrl, body.CancelUrl,
                idempotencyKey: idempotencyKey, ct: ct);

            context.Response.Headers["X-Stripe-Idempotency-Key"] = idempotencyKey;

            return err switch
            {
                SubscriptionError.None => HttpResults.Ok(new CheckoutResponse(checkoutUrl, sessionId, checkoutUrl)),
                SubscriptionError.InvalidPriceId => HttpResults.Json(
                    new ErrorResponse("invalid_price_id", msg!),
                    statusCode: StatusCodes.Status400BadRequest),
                SubscriptionError.PermissionDenied => HttpResults.Json(
                    new ErrorResponse("permission_denied", msg!),
                    statusCode: StatusCodes.Status403Forbidden),
                SubscriptionError.AlreadySubscribed => HttpResults.Json(
                    new ErrorResponse("already_subscribed", msg!),
                    statusCode: StatusCodes.Status409Conflict),
                SubscriptionError.StripeRateLimited => HttpResults.Json(
                    new ErrorResponse("stripe_rate_limited", msg!),
                    statusCode: StatusCodes.Status503ServiceUnavailable),
                _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        }

        // Legacy tier/interval/seat_count path.
        if (!OrgId.TryParse(body.OrgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        if (!Enum.TryParse<SubscriptionTier>(body.Tier, ignoreCase: true, out var tier))
            return HttpResults.Json(
                new ErrorResponse("invalid_tier", $"Invalid tier: {body.Tier}."),
                statusCode: StatusCodes.Status400BadRequest);

        var (legacyCheckoutUrl, legacySessionId, legacyErr, legacyMsg) = await svc.CreateCheckoutAsync(
            userId.Value, orgGuid, tier, body.Interval ?? "month",
            body.SeatCount ?? 1, body.SuccessUrl, body.CancelUrl, ct);

        return legacyErr switch
        {
            SubscriptionError.None => HttpResults.Ok(new CheckoutResponse(legacyCheckoutUrl, legacySessionId)),
            SubscriptionError.InvalidInterval => HttpResults.Json(
                new ErrorResponse("invalid_interval", legacyMsg!),
                statusCode: StatusCodes.Status400BadRequest),
            SubscriptionError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", legacyMsg!),
                statusCode: StatusCodes.Status403Forbidden),
            SubscriptionError.AlreadySubscribed => HttpResults.Json(
                new ErrorResponse("already_subscribed", legacyMsg!),
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> GetSubscription(
        string? org_id,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(org_id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (sub, tier) = await svc.GetForOrgAsync(userId.Value, orgGuid, ct);
        return HttpResults.Ok(new GetSubscriptionResponse(sub, tier));
    }

    private static async Task<IResult> UpdateSubscription(
        string id,
        [FromBody] UpdateSubscriptionRequest body,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!SubscriptionId.TryParse(id, out var subGuid))
            return HttpResults.Json(
                new ErrorResponse("subscription_not_found", "Subscription not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (sub, proration, err, msg) = await svc.UpdateAsync(
            userId.Value, subGuid, body.Tier, body.SeatCount, body.Interval, ct);

        return err switch
        {
            SubscriptionError.None => HttpResults.Ok(new UpdateSubscriptionResponse(sub!, proration!)),
            SubscriptionError.InvalidInterval => HttpResults.Json(
                new ErrorResponse("invalid_interval", msg!),
                statusCode: StatusCodes.Status400BadRequest),
            SubscriptionError.InvalidTier => HttpResults.Json(
                new ErrorResponse("invalid_tier", msg!),
                statusCode: StatusCodes.Status400BadRequest),
            SubscriptionError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", msg!),
                statusCode: StatusCodes.Status403Forbidden),
            SubscriptionError.DowngradeBlocked => HttpResults.Json(
                new ErrorResponse("subscription_downgrade_blocked", msg!),
                statusCode: StatusCodes.Status403Forbidden),
            SubscriptionError.SubscriptionNotFound => HttpResults.Json(
                new ErrorResponse("subscription_not_found", msg!),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> CancelSubscription(
        string id,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!SubscriptionId.TryParse(id, out var subGuid))
            return HttpResults.Json(
                new ErrorResponse("subscription_not_found", "Subscription not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (sub, err) = await svc.CancelAsync(userId.Value, subGuid, ct);

        return err switch
        {
            SubscriptionError.None => HttpResults.Ok(new CancelSubscriptionResponse(sub!, "Subscription will cancel at period end.")),
            SubscriptionError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can cancel subscriptions."),
                statusCode: StatusCodes.Status403Forbidden),
            SubscriptionError.SubscriptionNotFound => HttpResults.Json(
                new ErrorResponse("subscription_not_found", "Subscription not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ReactivateSubscription(
        string id,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!SubscriptionId.TryParse(id, out var subGuid))
            return HttpResults.Json(
                new ErrorResponse("subscription_not_found", "Subscription not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (sub, err) = await svc.ReactivateAsync(userId.Value, subGuid, ct);

        return err switch
        {
            SubscriptionError.None => HttpResults.Ok(new ReactivateSubscriptionResponse(sub!)),
            SubscriptionError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can reactivate subscriptions."),
                statusCode: StatusCodes.Status403Forbidden),
            SubscriptionError.SubscriptionNotFound => HttpResults.Json(
                new ErrorResponse("subscription_not_found", "Subscription not found."),
                statusCode: StatusCodes.Status404NotFound),
            SubscriptionError.NotCancelable => HttpResults.Json(
                new ErrorResponse("not_cancelable", "Subscription is not set to cancel at period end."),
                statusCode: StatusCodes.Status409Conflict),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> CreatePortal(
        [FromBody] PortalRequest body,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(body.OrgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (portalUrl, err) = await svc.CreatePortalAsync(userId.Value, orgGuid, body.ReturnUrl, ct);

        return err switch
        {
            SubscriptionError.None => HttpResults.Ok(new PortalResponse(portalUrl)),
            SubscriptionError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Only org owners can access the billing portal."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>
    /// POST /api/v1/subscriptions/billing-portal — creates a Stripe Billing Portal session for the
    /// authenticated user's owned/admin'd organization.
    /// Spec refs: docs/SPECIFICATION.md:7322-7345.
    /// </summary>
    private static async Task<IResult> CreateBillingPortal(
        [FromBody] BillingPortalRequest? body,
        HttpContext context,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        IOptions<AppOptions> appOptions,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var idempotencyKey = context.Request.Headers["Idempotency-Key"].FirstOrDefault()
                             ?? Guid.NewGuid().ToString();
        context.Response.Headers["X-Stripe-Idempotency-Key"] = idempotencyKey;

        var returnUrl = !string.IsNullOrWhiteSpace(body?.ReturnUrl)
            ? body!.ReturnUrl!
            : appOptions.Value.WebAppUrl.TrimEnd('/') + "/billing";

        try
        {
            var (portalUrl, err, msg) = await svc.CreateBillingPortalAsync(
                userId.Value, returnUrl, idempotencyKey, ct);

            return err switch
            {
                SubscriptionError.None => HttpResults.Ok(new BillingPortalResponse(portalUrl)),
                SubscriptionError.PermissionDenied => SubscriptionsProblem.PermissionDenied(context, msg!),
                SubscriptionError.NoBillingSetup => SubscriptionsProblem.NoBillingSetup(context),
                SubscriptionError.StripeRateLimited => SubscriptionsProblem.StripeRateLimited(context, idempotencyKey),
                _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        }
        catch (StripeUnavailableException ex)
        {
            return SubscriptionsProblem.StripeUnavailable(context, ex.RequestId, ex.IdempotencyKey);
        }
    }

    /// <summary>
    /// POST /api/v1/subscriptions/preview-proration — previews what changing the active
    /// subscription's price would charge or credit, without applying the change.
    /// Spec ref: docs/SPECIFICATION.md §"Update Subscription" (proration block shape).
    /// </summary>
    private static async Task<IResult> PreviewProration(
        [FromBody] PreviewProrationRequest body,
        HttpContext context,
        CurrentUserAccessor users,
        SubscriptionsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var idempotencyKey = context.Request.Headers["Idempotency-Key"].FirstOrDefault()
                             ?? Guid.NewGuid().ToString();
        context.Response.Headers["X-Stripe-Idempotency-Key"] = idempotencyKey;

        try
        {
            var (amount, renewal, credit, charge, err, msg) = await svc.PreviewProrationAsync(
                userId.Value, body.NewPriceId, body.SeatCount, idempotencyKey, ct);

            return err switch
            {
                SubscriptionError.None => HttpResults.Ok(
                    new PreviewProrationResponse(amount, renewal, credit, charge)),
                SubscriptionError.InvalidPriceId => HttpResults.Json(
                    new ErrorResponse("invalid_price_id", msg!),
                    statusCode: StatusCodes.Status400BadRequest),
                SubscriptionError.PermissionDenied => SubscriptionsProblem.PermissionDenied(context, msg!),
                SubscriptionError.NoActiveSubscription => SubscriptionsProblem.NoActiveSubscription(context),
                SubscriptionError.StripeRateLimited => SubscriptionsProblem.StripeRateLimited(context, idempotencyKey),
                _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        }
        catch (StripeUnavailableException ex)
        {
            return SubscriptionsProblem.StripeUnavailable(context, ex.RequestId, ex.IdempotencyKey);
        }
    }

    // Response types.
    // Legacy shape: checkout_url + session_id.
    // price_id path additionally emits url (= checkout_url) so jq .url works per the task observable.
    private sealed record CheckoutResponse(string CheckoutUrl, string SessionId, string? Url = null);
    private sealed record GetSubscriptionResponse(SubscriptionDto? Subscription, string Tier);
    private sealed record UpdateSubscriptionResponse(SubscriptionDto Subscription, ProrationResult Proration);
    private sealed record CancelSubscriptionResponse(SubscriptionDto Subscription, string Message);
    private sealed record ReactivateSubscriptionResponse(SubscriptionDto Subscription);
    private sealed record PortalResponse(string PortalUrl);
    private sealed record PreviewProrationResponse(int AmountDueNow, DateTime? RenewalDate, int Credit, int Charge);
}
