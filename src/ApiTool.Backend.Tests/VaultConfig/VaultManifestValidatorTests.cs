using System.Text.Json;
using ApiTool.Backend.VaultConfig;

namespace ApiTool.Backend.Tests.VaultConfig;

/// <summary>Tests for <see cref="VaultManifestValidator"/> literal-secret heuristic.</summary>
public sealed class VaultManifestValidatorTests
{
    [Theory]
    [InlineData(
        """{"team_secrets":{"vault_configs":{"prod":{"provider":"aws-secrets-manager","keys":{"api_key":"prod/api-key"}}}}}""",
        true,
        "coordinate-only value under api_key — ok")]
    [InlineData(
        """{"password":"abcdefghij1234567890"}""",
        false,
        "20-char alphanumeric under 'password' → literal secret")]
    [InlineData(
        """{"secret":"short"}""",
        true,
        "< 16 chars → not a secret-shape")]
    [InlineData(
        """{"token":"arn:aws:secretsmanager:us-east-1:123:secret/mything-AbCdEf"}""",
        true,
        "arn:* → recognized coordinate")]
    [InlineData(
        """{"api_key":"abcdefghij1234567890"}""",
        true,
        "field name not in {password,secret,token,key} → not flagged")]
    [InlineData(
        """{"nested":{"password":"1234567890abcdef"}}""",
        false,
        "16-char hex under nested password key → flagged")]
    public void Validate_flags_only_literal_secrets_under_sensitive_keys(
        string json, bool expectedOk, string reason)
    {
        var root = JsonDocument.Parse(json).RootElement;
        var result = VaultManifestValidator.Validate(root);
        result.Ok.Should().Be(expectedOk, because: reason);
    }

    [Fact]
    public void Validate_returns_dotted_paths_for_offending_keys()
    {
        const string json = """
            {
                "team_secrets": {
                    "vault_configs": {
                        "staging": {
                            "password": "abcdefgh12345678abcdefgh12345678"
                        }
                    }
                }
            }
            """;
        var root = JsonDocument.Parse(json).RootElement;
        var result = VaultManifestValidator.Validate(root);

        result.Ok.Should().BeFalse();
        result.OffendingPaths.Should().Contain("team_secrets.vault_configs.staging.password");
    }

    [Theory]
    [InlineData("""{"password":"vault://secret/mything"}""", true, "vault:// URI → coordinate")]
    [InlineData("""{"key":"azure-key-vault-name/some/path"}""", true, "contains / → coordinate")]
    [InlineData("""{"secret":"arn:aws:secretsmanager:us-east-1:123456789012:secret:name"}""", true, "arn: prefix → coordinate")]
    [InlineData("""{"token":"A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6"}""", false, "32-char alphanumeric token → flagged")]
    public void Validate_coordinate_exemption_covers_provider_patterns(string json, bool expectedOk, string reason)
    {
        var root = JsonDocument.Parse(json).RootElement;
        var result = VaultManifestValidator.Validate(root);
        result.Ok.Should().Be(expectedOk, because: reason);
    }

    [Fact]
    public void Validate_empty_object_is_ok()
    {
        var root = JsonDocument.Parse("{}").RootElement;
        var result = VaultManifestValidator.Validate(root);
        result.Ok.Should().BeTrue();
    }

    [Fact]
    public void Validate_collects_all_offending_paths_not_just_first()
    {
        const string json = """
            {
                "password": "abcdefghij1234567890",
                "secret": "1234567890abcdefghij"
            }
            """;
        var root = JsonDocument.Parse(json).RootElement;
        var result = VaultManifestValidator.Validate(root);
        result.OffendingPaths.Count.Should().Be(2);
    }
}
