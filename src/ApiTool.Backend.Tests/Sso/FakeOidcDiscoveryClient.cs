using ApiTool.Backend.Sso;
using Microsoft.IdentityModel.Protocols.OpenIdConnect;

namespace ApiTool.Backend.Tests.Sso;

/// <summary>
/// Deterministic in-process <see cref="IOidcDiscoveryClient"/> for unit and integration tests.
/// Scripts a configuration for each issuer URL via <see cref="SetConfig"/> or triggers failure via
/// <see cref="FailMode"/>.
/// </summary>
public sealed class FakeOidcDiscoveryClient : IOidcDiscoveryClient
{
    private readonly Dictionary<string, OpenIdConnectConfiguration> _configs
        = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>When <see langword="true"/>, every call throws <see cref="OidcDiscoveryException"/>.</summary>
    public bool FailMode { get; set; }

    /// <summary>Registers a scripted <see cref="OpenIdConnectConfiguration"/> for the given issuer URL.</summary>
    public void SetConfig(string issuerUrl, OpenIdConnectConfiguration config)
        => _configs[issuerUrl] = config;

    /// <inheritdoc/>
    public Task<OpenIdConnectConfiguration> GetConfigurationAsync(string issuerUrl, CancellationToken ct)
    {
        if (FailMode)
            throw new OidcDiscoveryException($"Fake discovery failure for {issuerUrl}");

        if (_configs.TryGetValue(issuerUrl, out var cfg))
            return Task.FromResult(cfg);

        throw new OidcDiscoveryException($"No scripted configuration for issuer '{issuerUrl}'");
    }
}
