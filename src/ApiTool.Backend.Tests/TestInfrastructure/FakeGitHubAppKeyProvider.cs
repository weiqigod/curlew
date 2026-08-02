using ApiTool.Backend.GitHub;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A deterministic <see cref="IGitHubAppKeyProvider"/> for tests.
/// Always returns a fixed JWT string without performing any real cryptography.
/// </summary>
public sealed class FakeGitHubAppKeyProvider(string fakeJwt, long appId = 12345L) : IGitHubAppKeyProvider
{
    /// <inheritdoc/>
    public Task<string> SignAppJwtAsync(GitHubAppJwtClaims claims, CancellationToken ct = default)
        => Task.FromResult(fakeJwt);

    /// <inheritdoc/>
    public long GetAppId() => appId;
}
