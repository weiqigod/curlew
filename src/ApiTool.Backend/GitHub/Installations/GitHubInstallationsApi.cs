// Refs docs/SPECIFICATION.md:8425 (daily reconciliation — GET /installation/repositories).
using System.Net.Http.Headers;
using System.Text.Json;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Real HTTP implementation of <see cref="IGitHubInstallationsApi"/>.
/// Exchanges the App JWT for an installation token (via <see cref="IInstallationTokenCache"/>)
/// and calls <c>GET /installation/repositories</c>.
/// </summary>
public sealed class GitHubInstallationsApi(
    IHttpClientFactory httpFactory,
    IInstallationTokenCache tokens,
    ILogger<GitHubInstallationsApi> log) : IGitHubInstallationsApi
{
    /// <inheritdoc/>
    public async Task<IReadOnlyList<RepoRef>> ListInstallationRepositoriesAsync(
        long installationId, CancellationToken ct)
    {
        var token = await tokens.GetOrRefreshAsync(installationId, ct).ConfigureAwait(false);

        var http = httpFactory.CreateClient("github-checks");
        using var req = new HttpRequestMessage(HttpMethod.Get, "/installation/repositories");
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        req.Headers.Accept.ParseAdd("application/vnd.github+json");
        req.Headers.Add("X-GitHub-Api-Version", "2022-11-28");

        using var rsp = await http.SendAsync(req, ct).ConfigureAwait(false);
        rsp.EnsureSuccessStatusCode();

        await using var stream = await rsp.Content.ReadAsStreamAsync(ct).ConfigureAwait(false);
        using var doc = await JsonDocument.ParseAsync(stream, cancellationToken: ct).ConfigureAwait(false);

        var arr = doc.RootElement.GetProperty("repositories");
        var list = new List<RepoRef>(arr.GetArrayLength());

        foreach (var r in arr.EnumerateArray())
        {
            var fullName = r.GetProperty("full_name").GetString()
                ?? throw new InvalidOperationException("GitHub repositories response missing 'full_name'");
            var slash = fullName.IndexOf('/');
            if (slash < 0)
            {
                log.LogWarning("Unexpected full_name without slash: {FullName}", fullName);
                continue;
            }

            var repoId = r.GetProperty("id").GetInt64();
            list.Add(new RepoRef(repoId, fullName[..slash], fullName[(slash + 1)..]));
        }

        log.LogDebug("Listed {Count} repositories for installation {InstallationId}", list.Count, installationId);
        return list;
    }
}
