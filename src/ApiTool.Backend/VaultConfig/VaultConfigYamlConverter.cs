using System.Text;
using System.Text.Json;
using YamlDotNet.Core;
using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Pure helper for converting between YAML and JSON representations of a vault-config
/// template. Uses <see href="https://github.com/aaubry/YamlDotNet">YamlDotNet</see>.
/// </summary>
public static class VaultConfigYamlConverter
{
    private static readonly IDeserializer YamlDeserializer =
        new DeserializerBuilder()
            .WithNamingConvention(NullNamingConvention.Instance)
            .Build();

    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        WriteIndented = false,
    };

    /// <summary>
    /// Parses a YAML string and returns the equivalent compact JSON string.
    /// Throws <see cref="YamlException"/> on malformed YAML.
    /// </summary>
    /// <param name="yaml">The YAML text to convert.</param>
    /// <returns>Compact JSON representation.</returns>
    public static string YamlToJson(string yaml)
    {
        var obj = YamlDeserializer.Deserialize<object?>(yaml);
        if (obj is null)
            return "{}";
        return JsonSerializer.Serialize(obj, JsonOptions);
    }
}
