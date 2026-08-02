using System.Text.RegularExpressions;

namespace ApiTool.Backend.Organizations;

/// <summary>Validates organization slugs against the allowed format rules.</summary>
public static partial class SlugValidator
{
    private const int MinLength = 2;
    private const int MaxLength = 100;

    [GeneratedRegex(@"^[a-z0-9]([a-z0-9\-]*[a-z0-9])?$")]
    private static partial Regex SlugPattern();

    /// <summary>
    /// Validates a slug against format rules.
    /// </summary>
    /// <param name="slug">The slug to validate.</param>
    /// <param name="error">Human-readable error message when validation fails; <see langword="null"/> on success.</param>
    /// <returns><see langword="true"/> if the slug is valid; otherwise <see langword="false"/>.</returns>
    public static bool TryValidate(string slug, out string? error)
    {
        if (string.IsNullOrEmpty(slug))
        {
            error = "Slug must not be empty.";
            return false;
        }

        if (slug.Length < MinLength)
        {
            error = $"Slug must be at least {MinLength} characters long.";
            return false;
        }

        if (slug.Length > MaxLength)
        {
            error = $"Slug must not exceed {MaxLength} characters.";
            return false;
        }

        if (!SlugPattern().IsMatch(slug))
        {
            error = "Slug must contain only lowercase letters, digits, and hyphens, and must not start or end with a hyphen.";
            return false;
        }

        error = null;
        return true;
    }
}
