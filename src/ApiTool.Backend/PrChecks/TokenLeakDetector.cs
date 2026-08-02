// Refs docs/SPECIFICATION.md:8643 (PRCHECK_TOKEN_LEAK_DETECTED defensive check).
using System.Text.RegularExpressions;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Detects GitHub installation tokens (ghs_…) in request bodies before deserialisation.
/// Refs docs/SPECIFICATION.md:8643.
/// </summary>
public static class TokenLeakDetector
{
    // Matches GitHub App installation tokens: ghs_ followed by 36+ alphanumeric characters.
    private static readonly Regex GhsToken = new(
        @"ghs_[A-Za-z0-9]{36,}",
        RegexOptions.Compiled | RegexOptions.CultureInvariant);

    /// <summary>
    /// Returns <c>true</c> if the raw request body contains a string that matches
    /// the GitHub installation token format (<c>ghs_[A-Za-z0-9]{36,}</c>).
    /// </summary>
    public static bool ContainsTokenLeak(string? rawBody) =>
        !string.IsNullOrEmpty(rawBody) && GhsToken.IsMatch(rawBody);
}
