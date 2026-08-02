using System.Text.RegularExpressions;

namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Enforces the spec invariant that all subscription-tier EF queries live exclusively under
/// <c>Internal/TierGates/</c> and the legacy <c>SsoTierGate.cs</c> adapter (M16-001).
/// See docs/SPECIFICATION.md "Tier-Gate Generic Abstraction (v4.3)".
/// </summary>
public class TierCheckLocalityTests
{
    // Matches: db.Subscriptions.Where(...).Select(s => (SubscriptionTier?)s.Tier)
    // Requires .Tier to appear *inside* the .Select(...) parentheses by bounding
    // the match from .Select( to the next ')' with .Tier in between. This correctly
    // handles the cast expression `(SubscriptionTier?)s.Tier)` while rejecting cases
    // where .Tier appears on a later line as a plain property assignment like `sub.Tier = ...`.
    private static readonly Regex TierProjectionPattern = new(
        @"db\.Subscriptions\s*\.Where\b[\s\S]{0,400}?\.Select\([\s\S]{0,100}?\.Tier[\s\S]{0,10}?\)",
        RegexOptions.Compiled);

    [Fact]
    public void Regex_positively_matches_known_tier_projection_pattern()
    {
        // Self-test: ensure the pattern matches the actual query shape used in TierGate.cs
        // so a broken regex cannot produce a false-passing locality test.
        const string knownViolator =
            """
            db.Subscriptions
                .Where(s => s.OrgId == orgId)
                .OrderByDescending(s => s.UpdatedAt)
                .Select(s => (SubscriptionTier?)s.Tier)
                .FirstOrDefaultAsync(ct)
            """;

        TierProjectionPattern.IsMatch(knownViolator).Should().BeTrue(
            because: "the locality regex must match the actual tier-projection query pattern " +
                     "or the enforcement test is a no-op");
    }

    [Fact]
    public void Subscription_tier_projection_lives_only_in_Internal_TierGates()
    {
        var backendRoot = ResolveBackendRoot();

        var allowed = new[]
        {
            Path.Combine(backendRoot, "Internal", "TierGates"),
            // Legacy adapter — delegates to TierGate.EnsureAsync; candidate for removal post-M16.
            Path.Combine(backendRoot, "Sso", "SsoTierGate.cs"),
        };

        var offenders = Directory
            .EnumerateFiles(backendRoot, "*.cs", SearchOption.AllDirectories)
            .Where(p => !allowed.Any(a => p.StartsWith(a, StringComparison.OrdinalIgnoreCase)))
            .Where(p => TierProjectionPattern.IsMatch(File.ReadAllText(p)))
            .ToList();

        offenders.Should().BeEmpty(
            because: "all subscription-tier checks must route through ITierGate per " +
                     "docs/SPECIFICATION.md Tier-Gate Generic Abstraction (v4.3). " +
                     $"Offending files:{Environment.NewLine}{string.Join(Environment.NewLine, offenders)}");
    }

    private static string ResolveBackendRoot()
    {
        // Walk up from the test assembly location to find src/ApiTool.Backend.
        var dir = new DirectoryInfo(
            Path.GetDirectoryName(typeof(TierCheckLocalityTests).Assembly.Location)!);

        while (dir is not null)
        {
            var candidate = Path.Combine(dir.FullName, "src", "ApiTool.Backend");
            if (Directory.Exists(candidate))
                return candidate;
            dir = dir.Parent;
        }

        throw new DirectoryNotFoundException(
            "Could not locate src/ApiTool.Backend relative to the test assembly. " +
            "Ensure the test is run from within the repository.");
    }
}
