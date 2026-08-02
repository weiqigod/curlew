using System.Text.RegularExpressions;

namespace ApiTool.Backend.Subscriptions;

/// <summary>
/// Validates that a Stripe price id belongs to the documented set of allowed prefixes.
/// Accepted prefixes: price_test_*, price_solo_*, price_team_*, price_professional_*, price_enterprise_*.
/// </summary>
public static partial class StripePriceAllowlist
{
    // Matches: price_(test|solo|team|professional|enterprise)_<at least one alphanumeric or underscore char>
    [GeneratedRegex(@"^price_(test|solo|team|professional|enterprise)_[a-zA-Z0-9_]+$", RegexOptions.None, matchTimeoutMilliseconds: 1000)]
    private static partial Regex AllowedPattern();

    /// <summary>Returns <see langword="true"/> when <paramref name="priceId"/> matches the documented allowlist pattern.</summary>
    public static bool IsAllowed(string? priceId)
    {
        if (string.IsNullOrEmpty(priceId))
            return false;
        return AllowedPattern().IsMatch(priceId);
    }
}
