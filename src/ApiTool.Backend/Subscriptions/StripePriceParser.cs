using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Maps a Stripe price id (drawn from <see cref="StripePriceAllowlist"/>) to the
/// (tier, default seat count, billing interval) triple it represents.
/// Shared between <see cref="SubscriptionsService"/> and test infrastructure.
/// </summary>
public static class StripePriceParser
{
    /// <summary>Parses an allowlisted price id; returns sensible defaults for unknown ids.</summary>
    public static (SubscriptionTier Tier, int DefaultSeatCount, string Interval) Parse(string priceId)
    {
        var tier = priceId switch
        {
            var p when p.StartsWith("price_test_solo_", StringComparison.Ordinal)
                    || p.StartsWith("price_solo_", StringComparison.Ordinal) => SubscriptionTier.Professional,
            var p when p.StartsWith("price_test_pro_", StringComparison.Ordinal)
                    || p.StartsWith("price_professional_", StringComparison.Ordinal) => SubscriptionTier.Professional,
            var p when p.StartsWith("price_test_team_", StringComparison.Ordinal)
                    || p.StartsWith("price_team_", StringComparison.Ordinal) => SubscriptionTier.Team,
            var p when p.StartsWith("price_enterprise_", StringComparison.Ordinal)
                    || p.StartsWith("price_test_enterprise_", StringComparison.Ordinal) => SubscriptionTier.Enterprise,
            _ => SubscriptionTier.Free,
        };
        var interval = priceId.Contains("yearly", StringComparison.OrdinalIgnoreCase) ? "year" : "month";
        var defaultSeatCount = tier == SubscriptionTier.Team ? 3 : 1;
        return (tier, defaultSeatCount, interval);
    }
}
