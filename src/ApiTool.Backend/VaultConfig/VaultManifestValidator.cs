using System.Text.Json;
using System.Text.RegularExpressions;

namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Inspects a parsed vault-config template for fields with sensitive key names
/// (<c>password</c>, <c>secret</c>, <c>token</c>, <c>key</c>) whose value
/// matches the literal-secret heuristic: <c>^[A-Za-z0-9+/=._-]{16,}$</c> and
/// is NOT a recognized provider coordinate pattern (ARN, URI with scheme, or a
/// path containing <c>:</c> or <c>/</c>). Spec refs: :5658, :5654-5715.
/// </summary>
public static partial class VaultManifestValidator
{
    [GeneratedRegex(@"^[A-Za-z0-9+/=._\-]{16,}$", RegexOptions.Compiled)]
    private static partial Regex LiteralSecretRegex();

    /// <summary>
    /// Values containing ':' or '/' or starting with 'arn:' are treated as provider
    /// coordinates (ARN, vault:// URI, azure key-vault reference, etc.) and are NOT flagged.
    /// </summary>
    [GeneratedRegex(@"^arn:|.+://|.+[:/]", RegexOptions.Compiled)]
    private static partial Regex CoordinateRegex();

    private static readonly HashSet<string> SensitiveKeyNames = new(StringComparer.OrdinalIgnoreCase)
    {
        "password", "secret", "token", "key",
    };

    /// <summary>
    /// Walks the parsed JSON tree and returns the dot-separated path of every suspect value.
    /// </summary>
    /// <param name="root">Root element of the parsed YAML/JSON template.</param>
    /// <returns>A <see cref="VaultValidationResult"/> with zero or more offending paths.</returns>
    public static VaultValidationResult Validate(JsonElement root)
    {
        var offending = new List<string>();
        Walk(root, string.Empty, offending);
        return new VaultValidationResult(offending);
    }

    private static void Walk(JsonElement element, string path, List<string> offending)
    {
        if (element.ValueKind == JsonValueKind.Object)
        {
            foreach (var property in element.EnumerateObject())
            {
                var childPath = string.IsNullOrEmpty(path)
                    ? property.Name
                    : $"{path}.{property.Name}";

                if (property.Value.ValueKind == JsonValueKind.String
                    && SensitiveKeyNames.Contains(property.Name))
                {
                    var value = property.Value.GetString() ?? string.Empty;
                    if (IsLiteralSecret(value))
                        offending.Add(childPath);
                }
                else
                {
                    Walk(property.Value, childPath, offending);
                }
            }
        }
        else if (element.ValueKind == JsonValueKind.Array)
        {
            var index = 0;
            foreach (var item in element.EnumerateArray())
            {
                Walk(item, $"{path}[{index++}]", offending);
            }
        }
    }

    private static bool IsLiteralSecret(string value)
    {
        if (!LiteralSecretRegex().IsMatch(value))
            return false;

        // Coordinate exemption: ARN, URI with scheme, or path-like with / or :
        if (CoordinateRegex().IsMatch(value))
            return false;

        return true;
    }
}
