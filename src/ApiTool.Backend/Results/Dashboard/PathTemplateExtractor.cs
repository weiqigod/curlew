using System.Text.RegularExpressions;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>
/// Extracts a normalised <c>path_template</c> from a request URL by replacing
/// id-shaped segments with <c>{id}</c>. Documented per spec line 10239:
/// path templates collapse <c>/users/4f3a...</c> and <c>/users/9c1e...</c>
/// into <c>/users/{id}</c>.
/// </summary>
/// <remarks>
/// Rules applied to each path segment:
/// <list type="bullet">
///   <item>Segment is all-digits ⇒ <c>{id}</c></item>
///   <item>Segment matches RFC 4122 UUID with hyphens ⇒ <c>{id}</c></item>
///   <item>Segment is 32-char lowercase hex (UUID without hyphens) ⇒ <c>{id}</c></item>
///   <item>Segment is hex-only and length ≥ 8 ⇒ <c>{id}</c></item>
///   <item>Query string is stripped</item>
///   <item>Trailing slash trimmed (except root)</item>
///   <item>Empty / unparseable input ⇒ <c>null</c> (caller excludes row from failures listing)</item>
/// </list>
/// </remarks>
public static partial class PathTemplateExtractor
{
    // RFC 4122 UUID with hyphens: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
    private static readonly Regex UuidWithHyphens =
        UuidWithHyphensRegex();

    // 32-char hex (UUID without hyphens)
    private static readonly Regex Hex32 =
        Hex32Regex();

    // All-hex string of length >= 8 (catches short UUIDs, object IDs, etc.)
    private static readonly Regex LongHex =
        LongHexRegex();

    // All-digit segment
    private static readonly Regex AllDigits =
        AllDigitsRegex();

    /// <summary>
    /// Extracts a normalised <c>path_template</c> from a request URL or path string.
    /// Returns <c>null</c> when the input is empty, whitespace, unparseable, or
    /// the method is null/empty.
    /// </summary>
    /// <param name="method">HTTP method (e.g. <c>"GET"</c>). Must already be uppercased by the caller (e.g. via <c>ToUpperInvariant()</c>); <see cref="Extract"/> does not normalise case.</param>
    /// <param name="requestUrl">Full URL or path-only string.</param>
    /// <returns>Normalised path template, or <c>null</c> when not derivable.</returns>
    public static string? Extract(string? method, string? requestUrl)
    {
        if (string.IsNullOrEmpty(method))
            return null;

        if (string.IsNullOrWhiteSpace(requestUrl))
            return null;

        // Try to parse as a full URI with explicit scheme (http/https) first.
        // Do NOT use UriKind.Absolute for path-only strings, because Uri.TryCreate
        // will happily parse "/users/123?q=v" as a file: URI and percent-encode the '?'.
        string path;
        var trimmed = requestUrl.Trim();
        if (Uri.TryCreate(trimmed, UriKind.Absolute, out var uri) && uri.IsAbsoluteUri
            && (uri.Scheme == Uri.UriSchemeHttp || uri.Scheme == Uri.UriSchemeHttps))
        {
            path = uri.AbsolutePath;
        }
        else if (trimmed.StartsWith('/'))
        {
            // Strip query string from plain path
            var q = trimmed.IndexOf('?', StringComparison.Ordinal);
            path = q >= 0 ? trimmed[..q] : trimmed;
        }
        else
        {
            // Unparseable (e.g. "not a url" without a leading slash)
            return null;
        }

        // Trim trailing slash (except root)
        if (path.Length > 1 && path.EndsWith('/'))
            path = path.TrimEnd('/');

        // Normalise each segment
        var segments = path.Split('/');
        for (var i = 0; i < segments.Length; i++)
        {
            var seg = segments[i];
            if (IsIdShaped(seg))
                segments[i] = "{id}";
        }

        return string.Join('/', segments);
    }

    /// <summary>Returns <see langword="true"/> when <paramref name="segment"/> is id-shaped.</summary>
    private static bool IsIdShaped(string segment)
    {
        if (string.IsNullOrEmpty(segment))
            return false;

        // All digits
        if (AllDigits.IsMatch(segment))
            return true;

        // RFC 4122 UUID with hyphens
        if (UuidWithHyphens.IsMatch(segment))
            return true;

        // 32-char hex (UUID without hyphens)
        if (Hex32.IsMatch(segment))
            return true;

        // Hex-only, length >= 8
        if (segment.Length >= 8 && LongHex.IsMatch(segment))
            return true;

        return false;
    }

    [GeneratedRegex(@"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$", RegexOptions.Compiled)]
    private static partial Regex UuidWithHyphensRegex();

    [GeneratedRegex(@"^[0-9a-fA-F]{32}$", RegexOptions.Compiled)]
    private static partial Regex Hex32Regex();

    [GeneratedRegex(@"^[0-9a-fA-F]+$", RegexOptions.Compiled)]
    private static partial Regex LongHexRegex();

    [GeneratedRegex(@"^\d+$", RegexOptions.Compiled)]
    private static partial Regex AllDigitsRegex();
}
