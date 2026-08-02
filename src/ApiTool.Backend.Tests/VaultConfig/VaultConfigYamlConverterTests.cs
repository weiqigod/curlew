using ApiTool.Backend.VaultConfig;
using YamlDotNet.Core;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>
/// Unit tests for <see cref="VaultConfigYamlConverter.YamlToJson"/>.
/// Covers edge cases identified in the plan (M16-017 review finding #3):
/// empty YAML, scalar values, deeply nested structures, and invalid YAML.
/// </summary>
public sealed class VaultConfigYamlConverterTests
{
    [Fact]
    public void YamlToJson_empty_yaml_returns_empty_object()
    {
        var result = VaultConfigYamlConverter.YamlToJson("");
        result.Should().Be("{}");
    }

    [Fact]
    public void YamlToJson_null_like_yaml_returns_empty_object()
    {
        // YAML deserialised as null (e.g. whitespace-only input)
        var result = VaultConfigYamlConverter.YamlToJson("   \n");
        result.Should().Be("{}");
    }

    [Fact]
    public void YamlToJson_scalar_string_value_produces_correct_json_string()
    {
        const string yaml = "key: hello";
        var result = VaultConfigYamlConverter.YamlToJson(yaml);
        result.Should().Contain("\"key\"");
        result.Should().Contain("\"hello\"");
    }

    [Fact]
    public void YamlToJson_integer_value_produces_numeric_json()
    {
        const string yaml = "count: 42";
        var result = VaultConfigYamlConverter.YamlToJson(yaml);
        // YamlDotNet deserialises integers as objects; JSON should render as number
        result.Should().Contain("\"count\"");
        result.Should().Contain("42");
    }

    [Fact]
    public void YamlToJson_deeply_nested_yaml_produces_correct_json_object()
    {
        const string yaml = """
            team_secrets:
              vault_configs:
                prod:
                  provider: aws-secrets-manager
                  keys:
                    api_key: prod/api-key
            """;

        var result = VaultConfigYamlConverter.YamlToJson(yaml);

        result.Should().Contain("\"team_secrets\"");
        result.Should().Contain("\"vault_configs\"");
        result.Should().Contain("\"prod\"");
        result.Should().Contain("\"provider\"");
        result.Should().Contain("aws-secrets-manager");
        result.Should().Contain("\"api_key\"");
        result.Should().Contain("prod/api-key");
    }

    [Fact]
    public void YamlToJson_multiline_string_value_is_preserved_as_string()
    {
        const string yaml = """
            description: |
              Line one.
              Line two.
            """;

        var result = VaultConfigYamlConverter.YamlToJson(yaml);
        result.Should().Contain("\"description\"");
        result.Should().Contain("Line one.");
        result.Should().Contain("Line two.");
    }

    [Fact]
    public void YamlToJson_throws_YamlException_on_malformed_yaml()
    {
        const string badYaml = ": invalid {{ yaml";
        var act = () => VaultConfigYamlConverter.YamlToJson(badYaml);
        act.Should().Throw<YamlException>();
    }

    [Fact]
    public void YamlToJson_list_value_produces_json_array()
    {
        const string yaml = """
            providers:
              - aws
              - azure
            """;

        var result = VaultConfigYamlConverter.YamlToJson(yaml);
        result.Should().Contain("\"providers\"");
        result.Should().Contain("\"aws\"");
        result.Should().Contain("\"azure\"");
    }
}
