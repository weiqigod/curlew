// Refs docs/SPECIFICATION.md:7990-8090 (Signing-Key Storage, kid format).
using System.Text.RegularExpressions;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Static helpers for the kid (key identifier) format defined in
/// docs/SPECIFICATION.md:8060 — <c>&lt;env&gt;-&lt;alg&gt;-&lt;YYYYMM&gt;-&lt;6char-uuid&gt;</c>.
/// All callers MUST validate untrusted kid input via <see cref="IsValid"/>
/// before any database lookup or filesystem access (RFC 8725 §3.10).
/// </summary>
public static class KeyId
{
    /// <summary>Allowlist regex from spec line 7999 / RFC 8725 §3.10.</summary>
    public static readonly Regex Allowlist = new(@"\A[a-z0-9-]{1,64}\z",
        RegexOptions.CultureInvariant | RegexOptions.Compiled);

    /// <summary>Returns true when <paramref name="kid"/> matches the allowlist regex.</summary>
    public static bool IsValid(string? kid) => kid is not null && Allowlist.IsMatch(kid);

    /// <summary>
    /// Generates a new kid using <paramref name="clock"/> for the YYYYMM segment.
    /// Format: <c>&lt;env&gt;-&lt;alg&gt;-&lt;YYYYMM&gt;-&lt;6hexchars&gt;</c>.
    /// </summary>
    /// <exception cref="InvalidOperationException">When the generated kid fails the allowlist regex (should not happen with valid env/alg inputs).</exception>
    public static string Generate(string env, string alg, TimeProvider clock)
    {
        var month = clock.GetUtcNow().ToString("yyyyMM");
        var suffix = Guid.NewGuid().ToString("N")[..6];
        var kid = $"{env}-{alg}-{month}-{suffix}";
        if (!IsValid(kid))
            throw new InvalidOperationException($"Generated kid '{kid}' fails allowlist regex");
        return kid;
    }
}
