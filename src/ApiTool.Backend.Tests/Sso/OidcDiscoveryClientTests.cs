using ApiTool.Backend.Sso;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>
/// Smoke tests for the production <see cref="OidcDiscoveryClient"/>.
/// These tests exercise the constructor path and the exception-wrapping
/// behaviour when the HTTP transport fails — without making real network calls.
/// </summary>
public sealed class OidcDiscoveryClientTests
{
    /// <summary>
    /// HTTP handler that always throws <see cref="HttpRequestException"/> to simulate
    /// a DNS failure or network error.
    /// </summary>
    private sealed class NetworkFailHandler : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
            => throw new HttpRequestException("Simulated network failure");
    }

    private static OidcDiscoveryClient BuildClient()
    {
        var services = new ServiceCollection();
        services.AddHttpClient("oidc")
            .ConfigurePrimaryHttpMessageHandler(() => new NetworkFailHandler());

        var sp = services.BuildServiceProvider();
        var factory = sp.GetRequiredService<IHttpClientFactory>();

        var options = Options.Create(new OidcOptions
        {
            DiscoveryCacheTtl = TimeSpan.FromMinutes(15),
        });

        return new OidcDiscoveryClient(factory, options);
    }

    [Fact]
    public async Task GetConfigurationAsync_wraps_http_failure_as_OidcDiscoveryException()
    {
        var client = BuildClient();

        await FluentActions.Invoking(() =>
                client.GetConfigurationAsync("http://localhost:9999/unreachable", default))
            .Should().ThrowAsync<OidcDiscoveryException>()
            .WithMessage("*OIDC discovery failed*");
    }

    [Fact]
    public async Task GetConfigurationAsync_wraps_invalid_operation_as_OidcDiscoveryException()
    {
        // A valid-looking HTTPS URL that ConfigurationManager will fail to fetch
        // in the test environment — triggers InvalidOperationException or HttpRequestException
        // depending on the platform. Both are caught and wrapped.
        var client = BuildClient();

        var ex = await Record.ExceptionAsync(() =>
            client.GetConfigurationAsync("http://localhost:9999/also-unreachable", default));

        ex.Should().NotBeNull()
            .And.BeOfType<OidcDiscoveryException>();
    }
}
