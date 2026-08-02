// Refs docs/SPECIFICATION.md:8425 (daily reconciliation — GET /installation/repositories).
using System.Net;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Unit tests for <see cref="GitHubInstallationsApi"/>.
/// Uses a <see cref="FakeHttpMessageHandler"/> to verify that:
///  - the Authorization header uses the installation token (not the App JWT), and
///  - the <c>full_name</c> field is parsed correctly into owner and name.
/// </summary>
public sealed class GitHubInstallationsApiTests
{
    private const long InstallationId = 99L;

    private static readonly string FakeInstallToken = "ghs_fakeinstall12345678901234567890123";

    // Minimal valid GET /installation/repositories response body.
    private static readonly string SingleRepoResponse = """
        {
          "total_count": 1,
          "repositories": [
            {
              "id": 1001,
              "full_name": "acme-corp/backend-api",
              "private": true
            }
          ]
        }
        """;

    // Response with multiple repos and one edge-case entry (no slash in full_name — should be skipped).
    private static readonly string MultiRepoResponse = """
        {
          "total_count": 3,
          "repositories": [
            { "id": 1, "full_name": "owner/repo-one" },
            { "id": 2, "full_name": "owner/repo-two" },
            { "id": 3, "full_name": "no-slash-repo" }
          ]
        }
        """;

    private static GitHubInstallationsApi BuildApi(
        FakeHttpMessageHandler repoHandler,
        string installationToken = "")
    {
        var token = string.IsNullOrEmpty(installationToken) ? FakeInstallToken : installationToken;
        var fakeCache = new FakeInstallationTokenCache(token);
        var factory = new FakeHttpClientFactory(repoHandler);
        return new GitHubInstallationsApi(factory, fakeCache, NullLogger<GitHubInstallationsApi>.Instance);
    }

    // ── Test: Authorization header must use the installation token, not the App JWT ──

    [Fact]
    public async Task List_AuthorizationHeaderUsesInstallationToken_NotAppJwt()
    {
        var handler = new FakeHttpMessageHandler(HttpStatusCode.OK, SingleRepoResponse);
        var api = BuildApi(handler, installationToken: FakeInstallToken);

        await api.ListInstallationRepositoriesAsync(InstallationId, CancellationToken.None);

        // The Authorization header must contain the installation token, not a JWT (App JWT).
        handler.LastAuthorizationHeader.Should().NotBeNull();
        handler.LastAuthorizationHeader.Should().Contain(FakeInstallToken,
            "the Authorization header must use the installation token (ghs_…), not the App JWT");
        // Sanity: must use Bearer scheme
        handler.LastAuthorizationHeader.Should().StartWith("Bearer ");
    }

    // ── Test: full_name is split correctly into owner and name ──

    [Fact]
    public async Task List_ParsesFullNameIntoOwnerAndName()
    {
        var handler = new FakeHttpMessageHandler(HttpStatusCode.OK, SingleRepoResponse);
        var api = BuildApi(handler);

        var repos = await api.ListInstallationRepositoriesAsync(InstallationId, CancellationToken.None);

        repos.Should().HaveCount(1);
        repos[0].Owner.Should().Be("acme-corp");
        repos[0].Name.Should().Be("backend-api");
        repos[0].Id.Should().Be(1001L);
    }

    // ── Test: multiple repos are all returned; entries without a slash are skipped ──

    [Fact]
    public async Task List_MultipleRepos_ReturnsAllValidAndSkipsMalformed()
    {
        var handler = new FakeHttpMessageHandler(HttpStatusCode.OK, MultiRepoResponse);
        var api = BuildApi(handler);

        var repos = await api.ListInstallationRepositoriesAsync(InstallationId, CancellationToken.None);

        // "no-slash-repo" has no slash so should be skipped
        repos.Should().HaveCount(2);
        repos.Should().Contain(r => r.Owner == "owner" && r.Name == "repo-one");
        repos.Should().Contain(r => r.Owner == "owner" && r.Name == "repo-two");
    }

    // ── Test: a single HTTP call is made per invocation ──

    [Fact]
    public async Task List_MakesExactlyOneHttpRequest()
    {
        var handler = new FakeHttpMessageHandler(HttpStatusCode.OK, SingleRepoResponse);
        var api = BuildApi(handler);

        await api.ListInstallationRepositoriesAsync(InstallationId, CancellationToken.None);

        handler.RequestCount.Should().Be(1);
    }

    // ── Minimal fake cache ───────────────────────────────────────────────────

    /// <summary>Fake token cache that always returns a fixed installation token.</summary>
    private sealed class FakeInstallationTokenCache(string token) : IInstallationTokenCache
    {
        public Task<string> GetOrRefreshAsync(long installationId, CancellationToken ct) =>
            Task.FromResult(token);

        public void Evict(long installationId) { }
    }
}
