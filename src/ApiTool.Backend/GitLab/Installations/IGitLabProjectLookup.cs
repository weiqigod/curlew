// Refs: M16-016 — project-path → numeric project_id resolution for PAT-based GitLab integration.
namespace ApiTool.Backend.GitLab.Installations;

/// <summary>
/// One-shot project-path → numeric project_id resolution against GitLab's
/// public API. Used during integration creation to validate the PAT + path
/// pair before persisting the row.
/// </summary>
public interface IGitLabProjectLookup
{
    /// <summary>
    /// Resolves <paramref name="projectPath"/> to a numeric <c>project_id</c> using the GitLab
    /// REST API at <paramref name="gitlabBaseUrl"/>. Authenticates with <paramref name="accessToken"/>
    /// via the <c>Private-Token</c> header.
    /// </summary>
    /// <param name="gitlabBaseUrl">Base URL of the GitLab instance (e.g. <c>https://gitlab.com</c>).</param>
    /// <param name="projectPath">Group-slash-project path (e.g. <c>group/project</c>).</param>
    /// <param name="accessToken">GitLab Project Access Token.</param>
    /// <param name="caBundlePem">Optional PEM CA bundle for self-managed instances using a private CA.</param>
    /// <param name="ct">Cancellation token.</param>
    Task<GitLabProjectLookupResult> LookupAsync(
        string gitlabBaseUrl,
        string projectPath,
        string accessToken,
        string? caBundlePem,
        CancellationToken ct);
}

/// <summary>Outcome of <see cref="IGitLabProjectLookup.LookupAsync"/>.</summary>
public sealed record GitLabProjectLookupResult(
    GitLabProjectLookupStatus Status,
    long? ProjectId,
    string? ResolvedPath);

/// <summary>Possible outcomes of a project-path lookup.</summary>
public enum GitLabProjectLookupStatus
{
    /// <summary>Lookup succeeded; <see cref="GitLabProjectLookupResult.ProjectId"/> is set.</summary>
    Ok,

    /// <summary>404 — project missing or PAT lacks access.</summary>
    NotFound,

    /// <summary>401 — PAT is invalid or revoked.</summary>
    Unauthorized,

    /// <summary><c>http://</c> base URL was rejected because <c>AllowHttp</c> is false.</summary>
    InsecureBaseUrl,

    /// <summary>Network or timeout error prevented the request from completing.</summary>
    Unreachable,

    /// <summary>2xx response received but body did not contain a numeric <c>id</c> field.</summary>
    InvalidResponse,

    /// <summary>
    /// The supplied <c>gitlab_base_url</c> is not a syntactically valid absolute URI
    /// (e.g. <c>"not-a-url"</c>). Returned instead of throwing <see cref="UriFormatException"/>.
    /// </summary>
    InvalidBaseUrl,
}
