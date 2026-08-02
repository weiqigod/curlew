// Refs docs/SPECIFICATION.md:8416-8421 (GitHub webhook payload shapes for installation events).
using System.Text.Json;
using ApiTool.Backend.GitHub.Installations;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Strongly-typed payload for the <c>installation.created</c> GitHub webhook event.
/// Refs docs/SPECIFICATION.md:8416-8417.
/// </summary>
public sealed record GithubInstallationCreatedPayload(
    long InstallationId,
    long AppId,
    string AccountLogin,
    string AccountType,
    string RepoSelection,
    IReadOnlyList<RepoRef> Repositories);

/// <summary>
/// Strongly-typed payload for <c>installation_repositories.added</c> and
/// <c>installation_repositories.removed</c> GitHub webhook events.
/// Refs docs/SPECIFICATION.md:8419-8421.
/// </summary>
public sealed record GithubInstallationRepositoriesPayload(
    long InstallationId,
    IReadOnlyList<RepoRef> Repositories);

/// <summary>
/// Strongly-typed payload for <c>installation.deleted</c>, <c>installation.suspend</c>,
/// and <c>installation.unsuspend</c> events. Spec :8560-8562.
/// </summary>
public sealed record GithubInstallationLifecyclePayload(long InstallationId);

/// <summary>
/// Strongly-typed payload for the <c>check_run.rerequested</c> event. Spec :8565.
/// </summary>
public sealed record GithubCheckRunRerequestedPayload(string ExternalId, string HeadSha);

/// <summary>
/// Extracts typed payloads from raw <see cref="JsonDocument"/> values.
/// Called by the dispatcher after signature verification.
/// </summary>
internal static class GithubWebhookPayloadParser
{
    /// <summary>Parses an <c>installation.created</c> payload.</summary>
    public static GithubInstallationCreatedPayload ParseInstallationCreated(JsonDocument doc)
    {
        var root = doc.RootElement;
        var install = root.GetProperty("installation");
        var installId = install.GetProperty("id").GetInt64();
        var appId = install.GetProperty("app_id").GetInt64();

        // account is at top-level in some payloads, or nested in installation
        string accountLogin;
        string accountType;
        if (root.TryGetProperty("account", out var topAccount))
        {
            accountLogin = topAccount.GetProperty("login").GetString() ?? string.Empty;
            accountType = topAccount.GetProperty("type").GetString() ?? "Organization";
        }
        else if (install.TryGetProperty("account", out var installAccount))
        {
            accountLogin = installAccount.GetProperty("login").GetString() ?? string.Empty;
            accountType = installAccount.GetProperty("type").GetString() ?? "Organization";
        }
        else
        {
            accountLogin = string.Empty;
            accountType = "Organization";
        }

        var repoSelection = root.TryGetProperty("repository_selection", out var rs)
            ? rs.GetString() ?? "selected"
            : "selected";

        var repos = new List<RepoRef>();
        if (root.TryGetProperty("repositories", out var reposEl))
        {
            foreach (var r in reposEl.EnumerateArray())
            {
                var id = r.TryGetProperty("id", out var idEl) ? idEl.GetInt64() : 0L;
                var fullName = r.TryGetProperty("full_name", out var fn) ? fn.GetString() ?? "" : "";
                var parts = fullName.Split('/', 2);
                var owner = parts.Length == 2 ? parts[0] : fullName;
                var name = parts.Length == 2 ? parts[1] : fullName;
                repos.Add(new RepoRef(id, owner, name));
            }
        }

        return new GithubInstallationCreatedPayload(
            installId, appId, accountLogin, accountType, repoSelection, repos);
    }

    /// <summary>Parses an <c>installation.deleted/suspend/unsuspend</c> payload.</summary>
    public static GithubInstallationLifecyclePayload ParseLifecycle(JsonDocument doc)
    {
        var installId = doc.RootElement.GetProperty("installation").GetProperty("id").GetInt64();
        return new GithubInstallationLifecyclePayload(installId);
    }

    /// <summary>
    /// Parses an <c>installation_repositories</c> payload. The <paramref name="action"/>
    /// selects which array (<c>repositories_added</c> for "added", <c>repositories_removed</c>
    /// for "removed") is returned as the <c>Repositories</c> list.
    /// </summary>
    public static GithubInstallationRepositoriesPayload ParseInstallationRepositories(
        JsonDocument doc, string? action)
    {
        var root = doc.RootElement;
        var installId = root.GetProperty("installation").GetProperty("id").GetInt64();

        var arrayKey = action == "removed" ? "repositories_removed" : "repositories_added";
        var repos = new List<RepoRef>();
        if (root.TryGetProperty(arrayKey, out var arr))
        {
            foreach (var r in arr.EnumerateArray())
            {
                var id = r.TryGetProperty("id", out var idEl) ? idEl.GetInt64() : 0L;
                var fullName = r.TryGetProperty("full_name", out var fn) ? fn.GetString() ?? "" : "";
                var parts = fullName.Split('/', 2);
                var owner = parts.Length == 2 ? parts[0] : fullName;
                var name = parts.Length == 2 ? parts[1] : fullName;
                repos.Add(new RepoRef(id, owner, name));
            }
        }

        return new GithubInstallationRepositoriesPayload(installId, repos);
    }

    /// <summary>Parses a <c>check_run.rerequested</c> payload.</summary>
    public static GithubCheckRunRerequestedPayload ParseCheckRunRerequested(JsonDocument doc)
    {
        var checkRun = doc.RootElement.GetProperty("check_run");
        var externalId = checkRun.TryGetProperty("external_id", out var ext)
            ? ext.GetString() ?? string.Empty
            : string.Empty;
        var headSha = checkRun.TryGetProperty("head_sha", out var sha)
            ? sha.GetString() ?? string.Empty
            : string.Empty;
        return new GithubCheckRunRerequestedPayload(externalId, headSha);
    }
}
