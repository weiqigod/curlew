// Refs docs/SPECIFICATION.md:8638-8642 (markdown safety for GitHub check-run output fields).
using System.Text;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Helpers for safe handling of user-controlled content in GitHub check-run Markdown fields.
/// Refs docs/SPECIFICATION.md:8638-8642.
/// </summary>
public static class MarkdownSafety
{
    /// <summary>GitHub's max field size for output.summary and output.text (60 000 characters).</summary>
    public const int MaxField = 60_000;

    private const string TruncationMarker = "(truncated)";

    /// <summary>
    /// Truncates <paramref name="input"/> to <see cref="MaxField"/> characters if it exceeds the limit,
    /// appending <c>(truncated)</c> to indicate the content was cut. Returns an empty string for null input.
    /// </summary>
    public static string Truncate(string? input)
    {
        if (string.IsNullOrEmpty(input)) return string.Empty;
        if (input.Length <= MaxField) return input;
        return string.Concat(
            input.AsSpan(0, MaxField - TruncationMarker.Length),
            TruncationMarker);
    }

    /// <summary>
    /// Escapes Markdown special characters in user-controlled interpolated strings
    /// (e.g., test names, file paths, commit messages) to prevent unintended formatting.
    /// Escaped characters: <c>* _ ` [ ] \</c>.
    /// </summary>
    internal static string EscapeUserContent(string input)
    {
        if (string.IsNullOrEmpty(input)) return input;
        var sb = new StringBuilder(input.Length + 16);
        foreach (var c in input)
        {
            if (c is '*' or '_' or '`' or '[' or ']' or '\\') sb.Append('\\');
            sb.Append(c);
        }
        return sb.ToString();
    }
}
