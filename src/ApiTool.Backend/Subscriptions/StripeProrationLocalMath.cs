using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Simple local proration math used by the sync <see cref="IStripeGateway.ComputeProration"/>
/// path (the PATCH apply path) and by <see cref="FakeStripeGateway"/>.
/// This is a full-month-delta approximation — it intentionally diverges from Stripe's
/// day-prorated math. Integration tests against stripe-mock exercise the real wire-protocol;
/// the smoke pass against real Stripe test mode covers cents-accuracy.
/// </summary>
internal static class StripeProrationLocalMath
{
    // Monthly price per seat in cents.
    private static readonly Dictionary<SubscriptionTier, int> s_monthlyCentsPerSeat = new()
    {
        [SubscriptionTier.Free] = 0,
        [SubscriptionTier.Professional] = 1900,
        [SubscriptionTier.Team] = 4900,
        [SubscriptionTier.Enterprise] = 9900,
    };

    /// <summary>Computes a full-month-delta proration for changing from one plan to another.</summary>
    public static ProrationResult Compute(
        SubscriptionTier fromTier, int fromSeats, SubscriptionTier toTier, int toSeats,
        string interval)
    {
        var multiplier = interval == "year" ? 12 : 1;
        var fromMonthly = s_monthlyCentsPerSeat.GetValueOrDefault(fromTier, 0) * fromSeats * multiplier;
        var toMonthly   = s_monthlyCentsPerSeat.GetValueOrDefault(toTier,   0) * toSeats   * multiplier;

        var delta = toMonthly - fromMonthly;

        if (delta >= 0)
            return new ProrationResult(Credit: 0, Charge: delta, Net: delta);
        else
        {
            var credit = -delta;
            return new ProrationResult(Credit: credit, Charge: 0, Net: -credit);
        }
    }
}
