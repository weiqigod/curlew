using System.Text.Json;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>Loads MJML markup and the JSON manifest for a template slug from disk.</summary>
public sealed class EmailTemplateLoader(string templatesRoot)
{
    /// <summary>Reads the raw MJML markup for the given slug.</summary>
    /// <exception cref="FileNotFoundException">Thrown when the <c>.mjml</c> file does not exist.</exception>
    public string LoadMjml(string slug)
    {
        var path = Path.Combine(templatesRoot, $"{slug}.mjml");
        if (!File.Exists(path))
            throw new FileNotFoundException($"MJML template missing: {path}", path);
        return File.ReadAllText(path);
    }

    /// <summary>Parses the JSON manifest sidecar for the given slug.</summary>
    /// <exception cref="FileNotFoundException">Thrown when the <c>.json</c> file does not exist.</exception>
    /// <exception cref="InvalidDataException">Thrown when the JSON is malformed or null.</exception>
    public EmailTemplateManifest LoadManifest(string slug)
    {
        var path = Path.Combine(templatesRoot, $"{slug}.json");
        if (!File.Exists(path))
            throw new FileNotFoundException($"Manifest missing: {path}", path);

        var json = File.ReadAllText(path);
        RawManifest? raw;
        try
        {
            raw = JsonSerializer.Deserialize<RawManifest>(json,
                new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
        }
        catch (JsonException ex)
        {
            throw new InvalidDataException($"Manifest {path} contains invalid JSON: {ex.Message}", ex);
        }

        if (raw is null)
            throw new InvalidDataException($"Manifest {path} is empty/invalid.");

        return new EmailTemplateManifest(
            raw.Slug ?? slug,
            raw.Subject ?? string.Empty,
            raw.Variables ?? new Dictionary<string, string>(),
            raw.TestData ?? new Dictionary<string, string>());
    }

    private sealed record RawManifest(
        [property: System.Text.Json.Serialization.JsonPropertyName("slug")]
        string? Slug,
        [property: System.Text.Json.Serialization.JsonPropertyName("subject")]
        string? Subject,
        [property: System.Text.Json.Serialization.JsonPropertyName("variables")]
        Dictionary<string, string>? Variables,
        [property: System.Text.Json.Serialization.JsonPropertyName("test_data")]
        Dictionary<string, string>? TestData);
}
