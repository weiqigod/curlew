using System.Text.Json;
using System.Text.Json.Serialization;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Typed view over the <c>organizations.settings</c> JSON column.
/// Only SSO-related keys are explicitly mapped; all other keys are round-tripped opaquely via
/// <see cref="Extras"/> so they are not silently dropped on write.
/// </summary>
public sealed class OrganizationSettings
{
    private static readonly JsonSerializerOptions SerializerOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    };

    /// <summary>Whether SSO login is enabled for this organisation.</summary>
    public bool SsoEnabled { get; set; }

    /// <summary>The SSO provider type: <c>"saml"</c>, <c>"oidc"</c>, or <see langword="null"/>.</summary>
    public string? SsoProvider { get; set; }

    /// <summary>SAML IdP configuration — set only when <see cref="SsoProvider"/> is <c>"saml"</c>.</summary>
    public SsoConfig? SsoConfig { get; set; }

    /// <summary>OIDC IdP configuration — set only when <see cref="SsoProvider"/> is <c>"oidc"</c>.</summary>
    public OidcConfig? OidcConfig { get; set; }

    /// <summary>
    /// All JSON keys that are not explicitly mapped by this class, preserved for round-trip fidelity.
    /// </summary>
    [JsonIgnore]
    public Dictionary<string, JsonElement> Extras { get; set; } = new(StringComparer.Ordinal);

    /// <summary>Deserializes a JSON blob from the database into an <see cref="OrganizationSettings"/> instance.</summary>
    /// <param name="json">The raw JSON string stored in <c>organizations.settings</c>.</param>
    /// <exception cref="ArgumentNullException">Thrown when <paramref name="json"/> is <see langword="null"/>.</exception>
    public static OrganizationSettings FromJson(string json)
    {
        ArgumentNullException.ThrowIfNull(json);

        using var doc = JsonDocument.Parse(json);
        var root = doc.RootElement;

        var settings = new OrganizationSettings();

        // Pass 1: harvest known scalar keys (sso_enabled, sso_provider) so that the
        // sso_config shape can be discriminated on provider in pass 2, regardless of
        // property order in the JSON document.
        string? rawSsoConfig = null;
        var extras = new Dictionary<string, JsonElement>(StringComparer.Ordinal);

        foreach (var prop in root.EnumerateObject())
        {
            switch (prop.Name)
            {
                case "sso_enabled":
                    settings.SsoEnabled = prop.Value.ValueKind == JsonValueKind.True;
                    break;
                case "sso_provider":
                    settings.SsoProvider = prop.Value.ValueKind == JsonValueKind.String
                        ? prop.Value.GetString()
                        : null;
                    break;
                case "sso_config":
                    if (prop.Value.ValueKind == JsonValueKind.Object)
                        rawSsoConfig = prop.Value.GetRawText();
                    break;
                default:
                    extras[prop.Name] = prop.Value.Clone();
                    break;
            }
        }

        settings.Extras = extras;

        // Pass 2: deserialise sso_config using the now-known provider.
        if (rawSsoConfig is not null)
        {
            if (settings.SsoProvider == "oidc")
                settings.OidcConfig = JsonSerializer.Deserialize<OidcConfig>(rawSsoConfig, SerializerOptions);
            else
                settings.SsoConfig = JsonSerializer.Deserialize<SsoConfig>(rawSsoConfig, SerializerOptions);
        }

        return settings;
    }

    /// <summary>Serializes this instance back to a JSON string suitable for storage in <c>organizations.settings</c>.</summary>
    /// <returns>A compact JSON string.</returns>
    public string ToJson()
    {
        using var ms = new System.IO.MemoryStream();
        using var writer = new Utf8JsonWriter(ms);

        writer.WriteStartObject();

        writer.WriteBoolean("sso_enabled", SsoEnabled);

        if (SsoProvider is not null)
            writer.WriteString("sso_provider", SsoProvider);

        if (SsoConfig is not null)
        {
            writer.WritePropertyName("sso_config");
            JsonSerializer.Serialize(writer, SsoConfig, SerializerOptions);
        }
        else if (OidcConfig is not null)
        {
            writer.WritePropertyName("sso_config");
            JsonSerializer.Serialize(writer, OidcConfig, SerializerOptions);
        }

        // Preserve all unrecognised keys intact.
        foreach (var kvp in Extras)
        {
            writer.WritePropertyName(kvp.Key);
            kvp.Value.WriteTo(writer);
        }

        writer.WriteEndObject();
        writer.Flush();

        return System.Text.Encoding.UTF8.GetString(ms.ToArray());
    }
}
