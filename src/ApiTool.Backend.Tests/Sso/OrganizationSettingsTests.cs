using ApiTool.Backend.Sso;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Tests for <see cref="OrganizationSettings"/> JSON serialization helpers.</summary>
public sealed class OrganizationSettingsTests
{
    [Theory]
    [InlineData("{}", false, null)]
    [InlineData("{\"sso_enabled\":true,\"sso_provider\":\"saml\"}", true, "saml")]
    [InlineData("{\"sso_enabled\":false,\"sso_provider\":null}", false, null)]
    public void FromJson_reads_sso_flags(string json, bool enabled, string? provider)
    {
        var settings = OrganizationSettings.FromJson(json);

        settings.SsoEnabled.Should().Be(enabled);
        settings.SsoProvider.Should().Be(provider);
    }

    [Fact]
    public void ToJson_round_trips_sso_config_and_preserves_unknown_keys()
    {
        var json = "{\"sso_enabled\":true,\"sso_provider\":\"saml\",\"custom_key\":\"custom_value\"}";

        var settings = OrganizationSettings.FromJson(json);
        settings.SsoEnabled.Should().BeTrue();

        var roundTripped = settings.ToJson();
        roundTripped.Should().Contain("\"sso_enabled\":true");
        roundTripped.Should().Contain("custom_key");
    }

    [Fact]
    public void ToJson_produces_snake_case_keys()
    {
        var settings = new OrganizationSettings
        {
            SsoEnabled = true,
            SsoProvider = "saml",
            SsoConfig = new SsoConfig(
                "https://idp.example.com/metadata",
                "https://sp.example.com/acs",
                "https://sp.example.com"),
        };

        var json = settings.ToJson();

        json.Should().Contain("\"sso_enabled\"");
        json.Should().Contain("\"sso_provider\"");
        json.Should().Contain("\"sso_config\"");
    }

    [Fact]
    public void FromJson_returns_empty_settings_for_empty_object()
    {
        var settings = OrganizationSettings.FromJson("{}");

        settings.SsoEnabled.Should().BeFalse();
        settings.SsoProvider.Should().BeNull();
        settings.SsoConfig.Should().BeNull();
    }

    [Fact]
    public void FromJson_reads_sso_config_when_present()
    {
        var json = """
            {
              "sso_enabled": true,
              "sso_provider": "saml",
              "sso_config": {
                "idp_metadata_url": "https://idp.example.com/metadata",
                "acs_url": "https://sp.example.com/acs",
                "entity_id": "https://sp.example.com"
              }
            }
            """;

        var settings = OrganizationSettings.FromJson(json);

        settings.SsoConfig.Should().NotBeNull();
        settings.SsoConfig!.IdpMetadataUrl.Should().Be("https://idp.example.com/metadata");
        settings.SsoConfig.AcsUrl.Should().Be("https://sp.example.com/acs");
        settings.SsoConfig.EntityId.Should().Be("https://sp.example.com");
    }

    // ── OIDC provider round-trip tests ────────────────────────────────────────

    [Fact]
    public void FromJson_reads_oidc_config_when_provider_is_oidc()
    {
        var json = """
            {"sso_enabled":true,"sso_provider":"oidc",
             "sso_config":{"issuer_url":"https://idp.example.com",
                           "client_id":"abc","redirect_uri":"http://localhost/cb",
                           "scopes":"openid email profile"}}
            """;

        var s = OrganizationSettings.FromJson(json);

        s.SsoProvider.Should().Be("oidc");
        s.OidcConfig.Should().NotBeNull();
        s.OidcConfig!.ClientId.Should().Be("abc");
        s.SsoConfig.Should().BeNull();
    }

    [Fact]
    public void ToJson_writes_oidc_under_sso_config_key_and_round_trips()
    {
        var s = new OrganizationSettings
        {
            SsoEnabled = true,
            SsoProvider = "oidc",
            OidcConfig = new OidcConfig("https://idp.example.com", "cid", "http://localhost/cb"),
        };

        var json = s.ToJson();
        json.Should().Contain("\"sso_config\"");
        json.Should().Contain("\"issuer_url\"");

        var r = OrganizationSettings.FromJson(json);
        r.OidcConfig.Should().NotBeNull();
        r.OidcConfig!.IssuerUrl.Should().Be("https://idp.example.com");
    }

    [Fact]
    public void Switching_provider_from_saml_to_oidc_clears_saml_config()
    {
        // Simulate a SAML settings JSON round-trip — OidcConfig should be null.
        var s = new OrganizationSettings
        {
            SsoEnabled = true,
            SsoProvider = "saml",
            SsoConfig = new SsoConfig("https://idp.example.com/metadata", "https://sp.example.com/acs", "https://sp.example.com"),
        };

        var json = s.ToJson();
        var r = OrganizationSettings.FromJson(json);

        r.OidcConfig.Should().BeNull();
        r.SsoConfig.Should().NotBeNull();
    }

    [Fact]
    public void FromJson_saml_provider_reads_saml_config_not_oidc_config()
    {
        var json = """
            {"sso_enabled":true,"sso_provider":"saml",
             "sso_config":{"idp_metadata_url":"https://idp.example.com/metadata",
                           "acs_url":"https://sp.example.com/acs",
                           "entity_id":"https://sp.example.com"}}
            """;

        var s = OrganizationSettings.FromJson(json);

        s.SsoConfig.Should().NotBeNull();
        s.OidcConfig.Should().BeNull();
    }

    [Fact]
    public void FromJson_unknown_provider_reads_saml_config_as_default()
    {
        // Backward compat: if sso_provider is absent/unknown, treat sso_config as SAML shape.
        var json = """
            {"sso_enabled":true,
             "sso_config":{"idp_metadata_url":"https://idp.example.com/metadata",
                           "acs_url":"https://sp.example.com/acs",
                           "entity_id":"https://sp.example.com"}}
            """;

        var s = OrganizationSettings.FromJson(json);

        s.SsoConfig.Should().NotBeNull();
        s.OidcConfig.Should().BeNull();
    }
}
