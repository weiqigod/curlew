using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Subscriptions;

/// <summary>Business logic for subscription creation, updates, and cancellation.</summary>
public sealed class SubscriptionsService(AppDbContext db, IStripeGateway stripe, TimeProvider clock, IAuditWriter audit)
{
    /// <summary>
    /// Creates a Stripe Checkout session for a new subscription.
    /// </summary>
    public async Task<(string checkoutUrl, string sessionId, SubscriptionError err, string? msg)>
        CreateCheckoutAsync(Guid userId, Guid orgId, SubscriptionTier tier, string interval,
                            int seatCount, string successUrl, string cancelUrl, CancellationToken ct)
    {
        // Validate interval.
        if (interval is not ("month" or "year"))
            return (string.Empty, string.Empty, SubscriptionError.InvalidInterval,
                $"Invalid interval '{interval}'. Must be \"month\" or \"year\".");

        // Verify user is owner of the org.
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null || member.Role != OrgRole.Owner)
            return (string.Empty, string.Empty, SubscriptionError.PermissionDenied, "Only org owners can manage subscriptions.");

        // Check for existing active subscription.
        var existing = await db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == orgId, ct);
        if (existing is not null)
            return (string.Empty, string.Empty, SubscriptionError.AlreadySubscribed, "Organization already has a subscription.");

        // Create Stripe session.
        var session = await stripe.CreateCheckoutSessionAsync(
            userId, orgId, tier, interval, seatCount, successUrl, cancelUrl, ct: ct);

        // Persist a subscription in Incomplete status.
        var now = clock.GetUtcNow().UtcDateTime;
        var sub = new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier,
            Status = SubscriptionStatus.Active, // Mark active immediately for fake Stripe.
            Interval = interval,
            SeatCount = seatCount,
            SeatLimit = seatCount,
            CurrentPeriodStart = now,
            CurrentPeriodEnd = interval == "year" ? now.AddYears(1) : now.AddMonths(1),
            StripeSubscriptionId = session.SessionId,
            CreatedAt = now,
            UpdatedAt = now,
        };
        db.Subscriptions.Add(sub);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "subscription.created",
            TargetType: "subscription",
            TargetId: sub.Id,
            Payload: new { tier = tier.ToString().ToLowerInvariant(), seat_count = seatCount }));

        await db.SaveChangesAsync(ct);

        return (session.CheckoutUrl, session.SessionId, SubscriptionError.None, null);
    }

    /// <summary>
    /// Creates a Stripe Checkout session using a Stripe price id.
    /// The org is resolved from the authenticated user's ownership; the request body cannot override it.
    /// </summary>
    public async Task<(string checkoutUrl, string sessionId, SubscriptionError err, string? msg)>
        CreateCheckoutByPriceAsync(Guid userId, string priceId,
                                   string successUrl, string cancelUrl,
                                   string idempotencyKey, CancellationToken ct)
    {
        // Validate price id against the allowlist (behaviour #4).
        if (!StripePriceAllowlist.IsAllowed(priceId))
            return (string.Empty, string.Empty, SubscriptionError.InvalidPriceId,
                $"Price '{priceId}' is not in the allowlist.");

        // Resolve the user's owned org (behaviour #6: bearer-validated org, not request body).
        var ownership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.UserId == userId && m.Role == OrgRole.Owner, ct);
        if (ownership is null)
            return (string.Empty, string.Empty, SubscriptionError.PermissionDenied,
                "User does not own any organization.");

        var orgId = ownership.OrgId;

        // Check for existing subscription (guard; future slices may loosen this).
        // NOTE: while it exists, the guard prevents the customer-id reuse path below from
        // ever finding a prior StripeCustomerId — any existing row would trigger AlreadySubscribed.
        // Customer-id reuse becomes meaningful once a future slice differentiates inactive rows
        // from active ones and loosens this guard. Until then we always pass null.
        var existingSub = await db.Subscriptions
            .FirstOrDefaultAsync(s => s.OrgId == orgId, ct);
        if (existingSub is not null)
            return (string.Empty, string.Empty, SubscriptionError.AlreadySubscribed,
                "Organization already has a subscription.");

        // No subscription row exists for this org, so no StripeCustomerId to reuse (behaviour #3).
        // When the AlreadySubscribed guard above is loosened in a future slice, this becomes
        // a live query:
        //   existingCustomerId = await db.Subscriptions
        //       .Where(s => s.OrgId == orgId && s.StripeCustomerId != null)
        //       .OrderByDescending(s => s.CreatedAt).Select(s => s.StripeCustomerId)
        //       .FirstOrDefaultAsync(ct);
        string? existingCustomerId = null;

        var (tier, seatCount, interval) = StripePriceParser.Parse(priceId);

        CheckoutSession session;
        try
        {
            session = await stripe.CreateCheckoutSessionAsync(
                userId, orgId, tier, interval, seatCount,
                successUrl, cancelUrl,
                priceId: priceId,
                existingCustomerId: existingCustomerId,
                idempotencyKey: idempotencyKey,
                ct: ct);
        }
        catch (StripeRateLimitedException)
        {
            return (string.Empty, string.Empty, SubscriptionError.StripeRateLimited,
                "Stripe rate-limited; retry with the same idempotency key.");
        }

        var now = clock.GetUtcNow().UtcDateTime;
        var sub = new Subscription
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Tier = tier,
            Status = SubscriptionStatus.Active,
            Interval = interval,
            SeatCount = seatCount,
            SeatLimit = seatCount,
            CurrentPeriodStart = now,
            CurrentPeriodEnd = interval == "year" ? now.AddYears(1) : now.AddMonths(1),
            StripeCustomerId = session.CustomerId,
            StripeSubscriptionId = session.SessionId,
            CreatedAt = now,
            UpdatedAt = now,
        };
        db.Subscriptions.Add(sub);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "subscription.created",
            TargetType: "subscription",
            TargetId: sub.Id,
            Payload: new { tier = tier.ToString().ToLowerInvariant(), seat_count = seatCount, price_id = priceId }));

        await db.SaveChangesAsync(ct);

        return (session.CheckoutUrl, session.SessionId, SubscriptionError.None, null);
    }

    /// <summary>Returns the active subscription for the given organization, or <see langword="null"/>.</summary>
    public async Task<(SubscriptionDto? sub, string tier)> GetForOrgAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return (null, "free");

        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == orgId, ct);
        if (sub is null)
            return (null, "free");

        // A canceled or quarantined subscription falls back to the free tier — the row is
        // retained for audit but the org no longer has paid entitlements.
        // Refs docs/SPECIFICATION.md:6800 (canceled), :6843 (quarantined).
        if (sub.Status is SubscriptionStatus.Canceled or SubscriptionStatus.Quarantined)
            return (ToDto(sub), "free");

        return (ToDto(sub), sub.Tier.ToString().ToLowerInvariant());
    }

    /// <summary>Updates seat count, tier, or interval on an existing subscription.</summary>
    public async Task<(SubscriptionDto? sub, ProrationResult? proration, SubscriptionError err, string? msg)>
        UpdateAsync(Guid userId, Guid subId, string? tierStr, int? seatCount, string? interval, CancellationToken ct)
    {
        // Validate interval when provided.
        if (interval is not null and not ("month" or "year"))
            return (null, null, SubscriptionError.InvalidInterval,
                $"Invalid interval '{interval}'. Must be \"month\" or \"year\".");

        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.Id == subId, ct);
        if (sub is null)
            return (null, null, SubscriptionError.SubscriptionNotFound, "Subscription not found.");

        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == sub.OrgId && m.UserId == userId, ct);
        if (member is null || member.Role != OrgRole.Owner)
            return (null, null, SubscriptionError.PermissionDenied, "Only org owners can update subscriptions.");

        var now = clock.GetUtcNow().UtcDateTime;

        // Check downgrade constraint.
        if (seatCount.HasValue)
        {
            var currentSeats = await CountSeatsAsync(sub.OrgId, now, ct);
            if (seatCount.Value < currentSeats)
                return (null, null, SubscriptionError.DowngradeBlocked,
                    $"Cannot reduce seat count to {seatCount.Value}; {currentSeats} seats are currently in use.");
        }

        // Validate tier when provided.
        SubscriptionTier? newTier = null;
        if (tierStr is not null)
        {
            if (!Enum.TryParse<SubscriptionTier>(tierStr, ignoreCase: true, out var parsedTier))
                return (null, null, SubscriptionError.InvalidTier, $"Invalid tier '{tierStr}'.");
            newTier = parsedTier;
        }

        var fromTier = sub.Tier;
        var fromSeats = sub.SeatLimit;

        // Apply updates.
        if (newTier.HasValue)
            sub.Tier = newTier.Value;
        if (seatCount.HasValue)
        {
            sub.SeatCount = seatCount.Value;
            sub.SeatLimit = seatCount.Value;
        }
        if (interval is not null)
            sub.Interval = interval;
        sub.UpdatedAt = now;

        var proration = stripe.ComputeProration(fromTier, fromSeats, sub.Tier, sub.SeatLimit, sub.Interval);

        string eventType;
        if (sub.Tier != fromTier || sub.SeatLimit != fromSeats)
            eventType = sub.Tier > fromTier || sub.SeatLimit > fromSeats ? "subscription.upgraded" : "subscription.downgraded";
        else
            eventType = "subscription.updated"; // interval-only change

        audit.Append(new AuditEvent(
            OrgId: sub.OrgId,
            ActorId: userId,
            EventType: eventType,
            TargetType: "subscription",
            TargetId: sub.Id,
            PreviousState: new { tier = fromTier.ToString().ToLowerInvariant(), seat_count = fromSeats },
            NewState: new { tier = sub.Tier.ToString().ToLowerInvariant(), seat_count = sub.SeatLimit }));

        await db.SaveChangesAsync(ct);
        return (ToDto(sub), proration, SubscriptionError.None, null);
    }

    /// <summary>Marks a subscription to cancel at the end of the current period.</summary>
    public async Task<(SubscriptionDto? sub, SubscriptionError err)> CancelAsync(Guid userId, Guid subId, CancellationToken ct)
    {
        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.Id == subId, ct);
        if (sub is null)
            return (null, SubscriptionError.SubscriptionNotFound);

        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == sub.OrgId && m.UserId == userId, ct);
        if (member is null || member.Role != OrgRole.Owner)
            return (null, SubscriptionError.PermissionDenied);

        var now = clock.GetUtcNow().UtcDateTime;
        sub.CancelAtPeriodEnd = true;
        sub.UpdatedAt = now;

        audit.Append(new AuditEvent(
            OrgId: sub.OrgId,
            ActorId: userId,
            EventType: "subscription.canceled",
            TargetType: "subscription",
            TargetId: subId));

        await db.SaveChangesAsync(ct);
        return (ToDto(sub), SubscriptionError.None);
    }

    /// <summary>Reactivates a subscription that was set to cancel at period end.</summary>
    public async Task<(SubscriptionDto? sub, SubscriptionError err)> ReactivateAsync(Guid userId, Guid subId, CancellationToken ct)
    {
        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.Id == subId, ct);
        if (sub is null)
            return (null, SubscriptionError.SubscriptionNotFound);

        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == sub.OrgId && m.UserId == userId, ct);
        if (member is null || member.Role != OrgRole.Owner)
            return (null, SubscriptionError.PermissionDenied);

        if (!sub.CancelAtPeriodEnd)
            return (null, SubscriptionError.NotCancelable);

        var now = clock.GetUtcNow().UtcDateTime;
        sub.CancelAtPeriodEnd = false;
        sub.UpdatedAt = now;

        await db.SaveChangesAsync(ct);
        return (ToDto(sub), SubscriptionError.None);
    }

    /// <summary>
    /// Creates a Stripe Billing Portal session for the authenticated user's owned/admin'd organization.
    /// Returns <see cref="SubscriptionError.NoBillingSetup"/> when the org has no <c>stripe_customer_id</c> on file.
    /// </summary>
    public async Task<(string portalUrl, SubscriptionError err, string? msg)>
        CreateBillingPortalAsync(Guid userId, string returnUrl, string idempotencyKey, CancellationToken ct)
    {
        // Resolve the user's owned/admin'd org (any membership at admin or owner role).
        var membership = await db.OrganizationMembers
            .Where(m => m.UserId == userId && (m.Role == OrgRole.Owner || m.Role == OrgRole.Admin))
            .OrderBy(m => m.JoinedAt) // deterministic when user belongs to multiple orgs
            .FirstOrDefaultAsync(ct);
        if (membership is null)
            return (string.Empty, SubscriptionError.PermissionDenied,
                "Only org owners or admins can access the billing portal.");

        var orgId = membership.OrgId;

        // Look up the most recent stripe_customer_id for the org.
        var customerId = await db.Subscriptions
            .Where(s => s.OrgId == orgId && s.StripeCustomerId != null)
            .OrderByDescending(s => s.CreatedAt)
            .Select(s => s.StripeCustomerId)
            .FirstOrDefaultAsync(ct);

        if (string.IsNullOrEmpty(customerId))
            return (string.Empty, SubscriptionError.NoBillingSetup,
                "Organization has no Stripe customer on file. Complete checkout first.");

        PortalSession session;
        try
        {
            session = await stripe.CreatePortalSessionAsync(
                userId, orgId, returnUrl,
                customerId: customerId,
                idempotencyKey: idempotencyKey,
                ct: ct);
        }
        catch (StripeRateLimitedException)
        {
            return (string.Empty, SubscriptionError.StripeRateLimited,
                "Stripe rate-limited; retry with the same idempotency key.");
        }
        // StripeUnavailableException is intentionally NOT caught here — the endpoint layer
        // maps it to a 502 ProblemDetails so the StripeException's RequestId is preserved.

        return (session.PortalUrl, SubscriptionError.None, null);
    }

    /// <summary>Creates a Stripe Billing Portal session for subscription self-management.</summary>
    public async Task<(string portalUrl, SubscriptionError err)> CreatePortalAsync(
        Guid userId, Guid orgId, string returnUrl, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null || member.Role != OrgRole.Owner)
            return (string.Empty, SubscriptionError.PermissionDenied);

        var session = await stripe.CreatePortalSessionAsync(userId, orgId, returnUrl, ct: ct);
        return (session.PortalUrl, SubscriptionError.None);
    }

    /// <summary>
    /// Returns the current seat count for an organization:
    /// active members + pending invitations (not accepted, not revoked, not expired).
    /// </summary>
    public Task<int> CountSeatsAsync(Guid orgId, DateTime now, CancellationToken ct)
        => SeatCounter.CountAsync(db, orgId, now, ct);

    /// <summary>
    /// Previews proration for swapping the active subscription's price to <paramref name="newPriceId"/>.
    /// Read-only — does not modify state and does not write the audit log.
    /// Behaviours: allowlist validation, no-op short-circuit when (tier, interval, seats) match,
    /// 409 when no active subscription, 200+amount_due_now=0 when Stripe reports no upcoming invoice.
    /// </summary>
    /// <remarks>
    /// NOTE: <c>StripeSubscriptionId</c> currently stores a checkout-session id (<c>cs_*</c>)
    /// set by <c>CreateCheckoutByPriceAsync</c> (M14-008). The live Stripe gateway passes it
    /// straight to Stripe; against stripe-mock this succeeds because mock accepts any subscription
    /// id-shaped string. This transitional behaviour closes when M14-011 webhooks attach the real
    /// <c>sub_*</c> id.
    /// </remarks>
    public async Task<(int AmountDueNow, DateTime? RenewalDate, int Credit, int Charge, SubscriptionError Err, string? Msg)>
        PreviewProrationAsync(Guid userId, string newPriceId, int? newSeatCount, string idempotencyKey, CancellationToken ct)
    {
        if (!StripePriceAllowlist.IsAllowed(newPriceId))
            return (0, null, 0, 0, SubscriptionError.InvalidPriceId,
                $"Price '{newPriceId}' is not in the allowlist.");

        // Resolve the user's owned org.
        var ownership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.UserId == userId && m.Role == OrgRole.Owner, ct);
        if (ownership is null)
            return (0, null, 0, 0, SubscriptionError.PermissionDenied,
                "Only org owners can preview proration.");

        var orgId = ownership.OrgId;

        var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == orgId, ct);
        if (sub is null || string.IsNullOrEmpty(sub.StripeSubscriptionId))
            return (0, null, 0, 0, SubscriptionError.NoActiveSubscription,
                "Organization has no active subscription.");

        var seats = newSeatCount ?? sub.SeatCount;
        var (newTier, _, newInterval) = StripePriceParser.Parse(newPriceId);

        // Behaviour #3: short-circuit when the requested target equals the current subscription's
        // (tier, interval, seats). Avoids a Stripe round-trip for pure no-op previews.
        // Note: without a stored price_id-to-row mapping, equality is derived from (tier, interval,
        // seat_count). This is a defensible simplification because StripePriceAllowlist emits one
        // price per (tier, interval) combination.
        if (sub.Tier == newTier && sub.Interval == newInterval && sub.SeatCount == seats)
            return (0, sub.CurrentPeriodEnd, 0, 0, SubscriptionError.None, null);

        try
        {
            var (result, renewal) = await stripe.ComputeProrationAsync(
                sub.StripeSubscriptionId!, newPriceId, seats,
                clock.GetUtcNow(), idempotencyKey, ct);

            return (result.Net, renewal, result.Credit, result.Charge, SubscriptionError.None, null);
        }
        catch (StripeRateLimitedException)
        {
            return (0, null, 0, 0, SubscriptionError.StripeRateLimited,
                "Stripe rate-limited; retry with the same idempotency key.");
        }
        // StripeUnavailableException is intentionally NOT caught here — the endpoint layer
        // maps it to a 502 ProblemDetails so the StripeException's RequestId is preserved.
    }

    private static SubscriptionDto ToDto(Subscription s) => new(
        Id: SubscriptionId.Format(s.Id),
        OrgId: OrgId.Format(s.OrgId),
        Tier: s.Tier.ToString().ToLowerInvariant(),
        Status: s.Status.ToString().ToLowerInvariant(),
        Interval: s.Interval,
        SeatCount: s.SeatCount,
        SeatLimit: s.SeatLimit,
        CurrentPeriodStart: s.CurrentPeriodStart,
        CurrentPeriodEnd: s.CurrentPeriodEnd,
        CancelAtPeriodEnd: s.CancelAtPeriodEnd,
        CreatedAt: s.CreatedAt);
}
