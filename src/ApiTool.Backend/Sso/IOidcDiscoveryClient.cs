using Microsoft.IdentityModel.Protocols.OpenIdConnect;

namespace ApiTool.Backend.Sso;

/// <summary>Fetches and caches OIDC provider metadata from a well-known discovery URL.</summary>
public interface IOidcDiscoveryClient
{
    /// <summary>
    /// Returns the IdP's <see cref="OpenIdConnectConfiguration"/> for the given issuer URL,
    /// fetching <c>{issuerUrl}/.well-known/openid-configuration</c> on cache miss.
    /// </summary>
    /// <param name="issuerUrl">The OIDC issuer base URL.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <exception cref="OidcDiscoveryException">
    /// Thrown when discovery fails (DNS, connection, HTTP 4xx/5xx, or invalid JSON).
    /// </exception>
    Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct);
}

/// <summary>Thrown when OIDC discovery fails for an issuer URL.</summary>
public sealed class OidcDiscoveryException(string message, Exception? inner = null)
    : Exception(message, inner);
