// Test double for IGitLabProjectLookup — injectable per-test.
// Refs: M16-016
using ApiTool.Backend.GitLab.Installations;

namespace ApiTool.Backend.Tests.GitLab.Installations;

/// <summary>
/// A scriptable <see cref="IGitLabProjectLookup"/> that returns a pre-configured result.
/// Register via <c>WithWebHostBuilder</c> per-test to control lookup behaviour.
/// </summary>
public sealed class FakeGitLabProjectLookup : IGitLabProjectLookup
{
    private GitLabProjectLookupResult _result = new(GitLabProjectLookupStatus.Ok, 42L, "group/project");

    /// <summary>Configures the result returned by the next call to <see cref="LookupAsync"/>.</summary>
    public void Returns(GitLabProjectLookupResult result) => _result = result;

    /// <summary>Configures an Ok result with the given project id.</summary>
    public void ReturnsOk(long projectId = 42L, string resolvedPath = "group/project") =>
        _result = new GitLabProjectLookupResult(GitLabProjectLookupStatus.Ok, projectId, resolvedPath);

    /// <inheritdoc />
    public Task<GitLabProjectLookupResult> LookupAsync(
        string gitlabBaseUrl,
        string projectPath,
        string accessToken,
        string? caBundlePem,
        CancellationToken ct) =>
        Task.FromResult(_result);
}
