using ApiTool.Backend.GitHub;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// Test double for <see cref="IInstallationTokenCache"/> that records Evict calls.
/// </summary>
public sealed class FakeInstallationTokenCache : IInstallationTokenCache
{
    /// <summary>Installation IDs passed to <see cref="Evict"/>.</summary>
    public List<long> Evictions { get; } = new();

    /// <inheritdoc/>
    public Task<string> GetOrRefreshAsync(long installationId, CancellationToken ct)
        => Task.FromResult($"fake-token-{installationId}");

    /// <inheritdoc/>
    public void Evict(long installationId) => Evictions.Add(installationId);
}
