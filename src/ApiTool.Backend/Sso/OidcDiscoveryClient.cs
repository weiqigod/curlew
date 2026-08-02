using System.Collections.Concurrent;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Protocols;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Production <see cref="IOidcDiscoveryClient"/> that fetches and caches OIDC discovery
/// documents using one <see cref="ConfigurationManager{T}"/> per issuer URL.
/// Cache is invalidated automatically at <see cref="OidcOptions.DiscoveryCacheTtl"/> intervals.
/// </summary>
public sealed class OidcDiscoveryClient(
    IHttpClientFactory httpClientFactory,
    IOptions<OidcOptions> options) : IOidcDiscoveryClient
{
    private readonly ConcurrentDictionary<string, ConfigurationManager<OpenIdConnectConfiguration>>
        _cache = new(StringComparer.OrdinalIgnoreCase);

    /// <inheritdoc/>
    public async Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct)
    {
        try
        {
            var mgr = _cache.GetOrAdd(issuerUrl, url =>
            {
                var http = httpClientFactory.CreateClient("oidc");
                var docRetriever = new HttpDocumentRetriever(http)
                {
                    // Allow HTTP only for localhost (dev/test); require HTTPS everywhere else.
                    RequireHttps = !url.StartsWith("http://localhost", StringComparison.OrdinalIgnoreCase),
                };

                return new ConfigurationManager<OpenIdConnectConfiguration>(
                    url.TrimEnd('/') + "/.well-known/openid-configuration",
                    new OpenIdConnectConfigurationRetriever(),
                    docRetriever)
                {
                    AutomaticRefreshInterval = options.Value.DiscoveryCacheTtl,
                    RefreshInterval = TimeSpan.FromMinutes(5),
                };
            });

            return await mgr.GetConfigurationAsync(ct).ConfigureAwait(false);
        }
        catch (Exception ex) when (ex is HttpRequestException or InvalidOperationException or TaskCanceledException)
        {
            throw new OidcDiscoveryException($"OIDC discovery failed for '{issuerUrl}': {ex.Message}", ex);
        }
    }
}
