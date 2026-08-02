using ApiTool.Backend.Sso;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>Sanity tests for <see cref="FakeOidcDiscoveryClient"/>.</summary>
public sealed class FakeOidcDiscoveryClientTests
{
    [Fact]
    public async Task Fake_returns_scripted_configuration()
    {
        var fake = new FakeOidcDiscoveryClient();
        fake.SetConfig("https://idp.example.com", new OpenIdConnectConfiguration
        {
            AuthorizationEndpoint = "https://idp.example.com/authorize",
            TokenEndpoint = "https://idp.example.com/token",
            Issuer = "https://idp.example.com",
        });

        var cfg = await fake.GetConfigurationAsync("https://idp.example.com", default);

        cfg.AuthorizationEndpoint.Should().Be("https://idp.example.com/authorize");
    }

    [Fact]
    public async Task Fake_throws_when_configured_to_fail()
    {
        var fake = new FakeOidcDiscoveryClient { FailMode = true };

        await FluentActions.Invoking(() => fake.GetConfigurationAsync("https://x", default))
            .Should().ThrowAsync<OidcDiscoveryException>();
    }

    [Fact]
    public async Task Fake_throws_for_unknown_issuer_url()
    {
        var fake = new FakeOidcDiscoveryClient();

        await FluentActions.Invoking(() => fake.GetConfigurationAsync("https://unknown.example.com", default))
            .Should().ThrowAsync<OidcDiscoveryException>()
            .WithMessage("*unknown.example.com*");
    }
}
