// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
// Handlers re-fetch the underlying object from Stripe — never trusting the event payload's
// snapshot, which is order-dependent. Idempotency is provided upstream by StripeWebhookStore.
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Webhooks.Handlers;

/// <summary>
/// Handles <c>customer.subscription.created</c>, <c>customer.subscription.updated</c>, and
/// <c>customer.subscription.deleted</c> webhook events by re-fetching the current Stripe state.
/// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
/// </summary>
public sealed class StripeSubscriptionHandler(
    AppDbContext db,
    IStripeGateway gateway,
    IEmailQueue emailQueue,
    TimeProvider clock,
    ILogger<StripeSubscriptionHandler> log)
{
    /// <summary>
    /// Handles <c>customer.subscription.created</c> and <c>customer.subscription.updated</c>.
    /// Re-fetches the Stripe Subscription by id; if 404 quarantines the local row.
    /// When <paramref name="isCreated"/> is <see langword="true"/> and the resulting subscription
    /// is non-free and active, preempts the org owner's active trial rows.
    /// Refs spec :5851 (tier-upgrade preemption), :6845–6848 (re-fetch pattern).
    /// </summary>
    /// <param name="subscriptionId">Stripe subscription id from the event payload.</param>
    /// <param name="isCreated">
    /// <see langword="true"/> for <c>customer.subscription.created</c>;
    /// <see langword="false"/> for <c>customer.subscription.updated</c>.
    /// Preemption fires only on <c>created</c>.
    /// </param>
    /// <param name="ct">Cancellation token.</param>
    public async Task HandleCreatedOrUpdatedAsync(string subscriptionId, bool isCreated, CancellationToken ct)
    {
        var stripeSub = await gateway.GetSubscriptionAsync(subscriptionId, ct);
        if (stripeSub is null)
        {
            // Subscription gone from Stripe (404). Quarantine the local row if any.
            var existing = await db.Subscriptions
                .FirstOrDefaultAsync(s => s.StripeSubscriptionId == subscriptionId, ct);
            if (existing is not null)
            {
                existing.Status = SubscriptionStatus.Quarantined;
                existing.UpdatedAt = clock.GetUtcNow().UtcDateTime;
                await db.SaveChangesAsync(ct);
                log.LogWarning(
                    "stripe_subscription_quarantined sub_id={SubscriptionId} reason=stripe_404",
                    subscriptionId);
            }
            return;
        }

        var customerId = stripeSub.CustomerId;

        // Find local row by subscription id first; fall back to customer id (pre-checkout row
        // has customer_id but not yet a subscription_id — the out-of-order case).
        var row = await db.Subscriptions
            .FirstOrDefaultAsync(
                s => s.StripeSubscriptionId == subscriptionId
                  || (s.StripeCustomerId == customerId && s.StripeSubscriptionId == null),
                ct);

        if (row is null)
        {
            // The first query already covers all lookup paths (by sub_id and by customer_id with no sub_id).
            // If both OR conditions miss, there is genuinely no local row for this subscription.
            log.LogWarning(
                "stripe_subscription_no_local_row sub_id={SubscriptionId} customer_id={CustomerId}",
                subscriptionId, customerId);
            return;
        }

        // Apply Stripe's current state (re-fetch wins — never apply a delta).
        var item = stripeSub.Items?.Data?.FirstOrDefault();
        var priceId = item?.Price?.Id ?? string.Empty;
        var (tier, _, interval) = StripePriceParser.Parse(priceId);

        row.StripeSubscriptionId = stripeSub.Id;
        row.Status = MapStatus(stripeSub.Status);
        row.Tier = tier;
        row.Interval = interval;
        if (item?.Quantity is long qty)
        {
            row.SeatCount = (int)qty;
            row.SeatLimit = (int)qty;
        }
        row.CurrentPeriodStart = stripeSub.CurrentPeriodStart;
        row.CurrentPeriodEnd = stripeSub.CurrentPeriodEnd;
        row.CancelAtPeriodEnd = stripeSub.CancelAtPeriodEnd;
        row.UpdatedAt = clock.GetUtcNow().UtcDateTime;
        await db.SaveChangesAsync(ct);

        // M16-006: preempt owner's active trials on the first creation of a paid subscription.
        // Refs spec :5851 (tier-upgrade preemption rule).
        if (isCreated && row.Tier != SubscriptionTier.Free && row.Status == SubscriptionStatus.Active)
            await PreemptOwnerTrialsAsync(row.OrgId, ct);
    }

    /// <summary>
    /// Transitions all active trial rows of the given org's <see cref="OrgRole.Owner"/> to
    /// <see cref="TrialKind.PreemptedBySubscription"/> with <c>expires_at = now()</c>.
    /// Refs spec :5851 (transition rule), :11030 (UPDATE pattern).
    /// Multi-org scope: only the owner's trials preempt — other org members keep theirs.
    /// </summary>
    private async Task PreemptOwnerTrialsAsync(Guid orgId, CancellationToken ct)
    {
        var ownerId = await db.OrganizationMembers
            .Where(m => m.OrgId == orgId && m.Role == OrgRole.Owner)
            .Select(m => (Guid?)m.UserId)
            .FirstOrDefaultAsync(ct);
        if (ownerId is null) return;

        var now = clock.GetUtcNow().UtcDateTime;

        // Load + update active trial rows. Bounded by |TrialFeatures| so the
        // load+update is O(few) — no need for ExecuteUpdateAsync.
        var rows = await db.Trials
            .Where(t => t.UserId == ownerId
                     && t.ConsumedAt == null
                     && t.ExpiresAt > now
                     && t.Kind != TrialKind.PreemptedBySubscription)
            .ToListAsync(ct);

        if (rows.Count == 0) return;

        foreach (var r in rows)
        {
            r.Kind = TrialKind.PreemptedBySubscription;
            r.ExpiresAt = now;
            r.UpdatedAt = now;
        }
        await db.SaveChangesAsync(ct);

        log.LogInformation(
            "trial_preempted_by_subscription user_id={UserId} org_id={OrgId} count={Count}",
            ownerId, orgId, rows.Count);
    }

    /// <summary>
    /// Handles <c>customer.subscription.deleted</c>. Marks the local row <c>Canceled</c>,
    /// clears the subscription id, and enqueues a <c>billing_subscription_canceled</c> email.
    /// Re-fetches first; proceeds to cancel even if Stripe returns 404 (object already gone).
    /// </summary>
    public async Task HandleDeletedAsync(string subscriptionId, CancellationToken ct)
    {
        // Re-fetch even on delete — Stripe still returns the (canceled) subscription for a window.
        var stripeSub = await gateway.GetSubscriptionAsync(subscriptionId, ct);

        var row = await db.Subscriptions
            .FirstOrDefaultAsync(s => s.StripeSubscriptionId == subscriptionId, ct);
        if (row is null)
        {
            log.LogInformation(
                "stripe_subscription_deleted_no_local_row sub_id={SubscriptionId}", subscriptionId);
            return;
        }

        var now = clock.GetUtcNow();
        row.Status = SubscriptionStatus.Canceled;
        row.CanceledAt = stripeSub?.CanceledAt ?? now.UtcDateTime;
        row.StripeSubscriptionId = null; // cleared per behavior #3
        row.UpdatedAt = now.UtcDateTime;

        // Compose and enqueue cancellation email to the org owner.
        var owner = await db.Users
            .Where(u => u.Id == db.OrganizationMembers
                .Where(m => m.OrgId == row.OrgId && m.Role == OrgRole.Owner)
                .Select(m => m.UserId)
                .FirstOrDefault())
            .Select(u => new { u.Email })
            .FirstOrDefaultAsync(ct);

        if (owner is not null)
        {
            var at = owner.Email.IndexOf('@', StringComparison.Ordinal);
            var firstName = at > 0 ? owner.Email[..at] : owner.Email;
            await emailQueue.EnqueueAsync(new EmailMessage(
                To: owner.Email,
                TemplateSlug: "billing_subscription_canceled",
                Variables: new Dictionary<string, string>
                {
                    ["first_name"] = firstName,
                    ["tier"] = row.Tier.ToString(),
                    ["effective_date"] = (stripeSub?.CanceledAt ?? now.UtcDateTime).ToString("yyyy-MM-dd"),
                },
                EnqueuedAt: now), ct);
        }
        else
        {
            log.LogWarning(
                "stripe_subscription_canceled_no_owner org_id={OrgId} sub_id={SubscriptionId} — cancellation email not sent",
                row.OrgId, subscriptionId);
        }

        await db.SaveChangesAsync(ct);
    }

    /// <summary>Maps a Stripe subscription status string to the local enum value.</summary>
    internal SubscriptionStatus MapStatus(string stripeStatus)
    {
        var status = stripeStatus switch
        {
            "active" or "trialing"               => SubscriptionStatus.Active,
            "past_due"                           => SubscriptionStatus.PastDue,
            "canceled"                           => SubscriptionStatus.Canceled,
            "incomplete" or "incomplete_expired" => SubscriptionStatus.Incomplete,
            "unpaid" or "paused"                 => SubscriptionStatus.PastDue, // closest local equivalent
            _ => SubscriptionStatus.Incomplete,
        };

        if (status == SubscriptionStatus.Incomplete
            && stripeStatus is not "incomplete" and not "incomplete_expired")
        {
            log.LogWarning(
                "stripe_subscription_unknown_status status={Status} — mapping to Incomplete",
                stripeStatus);
        }

        return status;
    }
}
