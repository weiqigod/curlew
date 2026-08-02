// Refs docs/SPECIFICATION.md:8429-8550 (outbound GitHub Checks API POST).
using System.Net;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.Json.Nodes;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Posts check runs to the GitHub Checks API on behalf of an organisation's
/// GitHub App installation. Handles two-stage auth (App JWT → installation token),
/// in-process token caching, rate-limit tracking, and retry idempotency.
/// Refs docs/SPECIFICATION.md:8429-8550.
/// </summary>
public sealed class CheckRunPoster(
    AppDbContext db,
    IInstallationTokenCache tokens,
    IRateLimitTracker rateLimit,
    IHttpClientFactory httpFactory,
    IGitHubAppKeyProvider keyProvider,
    TimeProvider clock,
    ILogger<CheckRunPoster> log) : ICheckRunPoster
{
    private static readonly JsonSerializerOptions JsonOpts = new(JsonSerializerDefaults.Web);
    private static readonly System.Text.RegularExpressions.Regex ScrubPattern = new(
        @"ghs_[A-Za-z0-9]{36,}",
        System.Text.RegularExpressions.RegexOptions.Compiled | System.Text.RegularExpressions.RegexOptions.CultureInvariant);

    /// <summary>
    /// Posts a check run for the given <c>pr_checks</c> row. Updates the row in place
    /// (<c>posting_started_at</c>, <c>posted_at</c>, <c>check_run_id</c>, <c>status</c>,
    /// <c>last_error</c>, <c>attempt_count</c>).
    /// </summary>
    public async Task<CheckRunPostResult> PostAsync(Guid prCheckId, CancellationToken ct)
    {
        // Step 1: Load the PrCheck row
        var row = await db.PrChecks.FindAsync([prCheckId], ct)
            ?? throw new ArgumentException($"PrCheck {prCheckId} not found", nameof(prCheckId));

        // Step 2: Load a live installation for this org
        var install = await db.GithubInstallations
            .Where(i => i.OrgId == row.OrgId)
            .OrderByDescending(i => i.InstalledAt)
            .FirstOrDefaultAsync(ct);

        if (install is null)
        {
            log.LogWarning("No GitHub installation found for org {OrgId}", row.OrgId);
            return Failed(PrCheckErrorCode.NoInstallation, "No GitHub App installation found for this organisation.");
        }

        // Step 3: Check suspension
        if (install.SuspendedAt.HasValue)
        {
            log.LogWarning("Installation {InstallationId} is suspended", install.InstallationId);
            return Failed(PrCheckErrorCode.InstallationSuspended, "GitHub App installation is suspended.");
        }

        // Step 4: Check deletion
        if (install.DeletedAt.HasValue)
        {
            log.LogWarning("Installation {InstallationId} was deleted", install.InstallationId);
            return Failed(PrCheckErrorCode.InstallationDeleted, "GitHub App installation was deleted.");
        }

        // Step 5: Check repo coverage
        if (!IsRepoCovered(row.Repo, install.RepoSelection, install.RepoSetJson))
        {
            log.LogWarning("Repo {Repo} not covered by installation {InstallationId}", row.Repo, install.InstallationId);
            return Failed(PrCheckErrorCode.RepoNotCovered, $"Repository '{row.Repo}' is not covered by the GitHub App installation.");
        }

        // Compute conclusion early — needed by both the idempotency path and the POST path.
        var conclusion = PrCheckConclusionMapper.TryMap(row.State, out var mappedConclusion)
            ? mappedConclusion
            : "failure"; // fallback

        // Step 6: Idempotency GET (crash-recovery path per spec :8529-8533)
        // If posting started but never completed (PostingStartedAt set, PostedAt null),
        // check GitHub for an existing run with our external_id before retrying.
        if (row.PostingStartedAt.HasValue && !row.PostedAt.HasValue)
        {
            var idempotencyResult = await TryFindExistingCheckRunAsync(row, install, conclusion, ct);
            if (idempotencyResult is not null)
                return idempotencyResult;
        }

        // Step 7: Check rate-limit
        if (rateLimit.IsBlocked(install.InstallationId))
        {
            log.LogInformation("Rate-limited for installation {InstallationId}, queuing", install.InstallationId);
            row.Status = "queued";
            await db.SaveChangesAsync(ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null, "Rate-limited by GitHub.", PrCheckErrorCode.GithubRateLimited);
        }

        // Step 8: Fetch installation token
        string installToken;
        try
        {
            installToken = await tokens.GetOrRefreshAsync(install.InstallationId, ct);
        }
        catch (Exception ex)
        {
            log.LogError(ex, "Failed to fetch installation token for installation {InstallationId}", install.InstallationId);
            return await RecordTransientFailure(row, "Failed to fetch GitHub installation token.", ct);
        }

        // Step 9: Record posting started
        var now = clock.GetUtcNow().UtcDateTime;
        row.PostingStartedAt = now;
        row.AttemptCount++;
        row.InstallationId = install.InstallationId;
        row.Status = "posting";
        await db.SaveChangesAsync(ct);

        // Step 10: Truncate markdown fields (only when present)
        if (row.OutputSummary is not null)
            row.OutputSummary = MarkdownSafety.Truncate(row.OutputSummary);
        if (row.OutputText is not null)
            row.OutputText = MarkdownSafety.Truncate(row.OutputText);

        // Build request body
        var (owner, repoName) = SplitRepo(row.Repo);

        var payload = BuildPayload(row, owner, repoName, conclusion, now);

        // Step 11: POST to GitHub
        var http = httpFactory.CreateClient("github-checks");
        using var req = new HttpRequestMessage(
            HttpMethod.Post,
            $"/repos/{owner}/{repoName}/check-runs");
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", installToken);
        req.Headers.Accept.ParseAdd("application/vnd.github+json");
        req.Headers.Add("X-GitHub-Api-Version", "2022-11-28");
        req.Content = new StringContent(payload, Encoding.UTF8, "application/json");

        HttpResponseMessage rsp;
        try
        {
            rsp = await http.SendAsync(req, ct);
        }
        catch (Exception ex)
        {
            log.LogError(ex, "Network error posting check run for PrCheck {PrCheckId}", prCheckId);
            return await RecordTransientFailure(row, "Network error contacting GitHub.", ct);
        }

        using var _ = rsp;

        // Step 12: Observe rate-limit headers
        ObserveRateLimitHeaders(rsp, install.InstallationId);

        // Step 13: Handle 2xx
        if (rsp.IsSuccessStatusCode)
        {
            var responseBody = await rsp.Content.ReadAsStringAsync(ct);
            long? checkRunId = null;
            try
            {
                using var doc = JsonDocument.Parse(responseBody);
                if (doc.RootElement.TryGetProperty("id", out var idEl))
                    checkRunId = idEl.GetInt64();
            }
            catch (JsonException ex)
            {
                log.LogWarning(ex, "Could not parse GitHub check-runs response for PrCheck {PrCheckId}", prCheckId);
            }

            row.CheckRunId = checkRunId;
            row.PostedAt = clock.GetUtcNow().UtcDateTime;
            row.Status = "posted";
            row.Conclusion = conclusion;

            // Persist truncated fields
            await db.SaveChangesAsync(ct);

            log.LogInformation("Posted check run {CheckRunId} for PrCheck {PrCheckId}", checkRunId, prCheckId);
            return new CheckRunPostResult(PrCheckPostStatus.Posted, checkRunId, null, null);
        }

        // Step 14–15: Handle errors
        var errorBody = await rsp.Content.ReadAsStringAsync(ct);
        log.LogWarning("GitHub returned {StatusCode} for PrCheck {PrCheckId}: {Body}",
            (int)rsp.StatusCode, prCheckId, ScrubTokens(errorBody));

        // Handle rate-limit 403 with Retry-After
        if (rsp.StatusCode == HttpStatusCode.Forbidden &&
            rsp.Headers.TryGetValues("Retry-After", out var retryAfterValues) &&
            int.TryParse(retryAfterValues.FirstOrDefault(), out var retrySeconds))
        {
            rateLimit.Update(install.InstallationId, null, retrySeconds, null);
            row.Status = "queued";
            row.LastError = $"GitHub secondary rate-limit, retry after {retrySeconds}s.";
            await db.SaveChangesAsync(ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null, row.LastError, PrCheckErrorCode.GithubRateLimited);
        }

        // Handle 4xx permanent failures
        // 404: repository not found in the installation → RepoNotCovered
        // 422: payload is malformed → PermanentFailure (not a repo-access issue)
        if (rsp.StatusCode == HttpStatusCode.NotFound)
        {
            row.Status = "failed";
            row.LastError = $"GitHub returned {(int)rsp.StatusCode}.";
            await db.SaveChangesAsync(ct);
            return Failed(PrCheckErrorCode.RepoNotCovered, row.LastError);
        }

        if (rsp.StatusCode == HttpStatusCode.UnprocessableEntity)
        {
            row.Status = "failed";
            row.LastError = $"GitHub returned {(int)rsp.StatusCode} (unprocessable entity — payload may be malformed).";
            await db.SaveChangesAsync(ct);
            return Failed(PrCheckErrorCode.PermanentFailure, row.LastError);
        }

        // Step 15: 5xx / other transient
        return await RecordTransientFailure(row, $"GitHub returned {(int)rsp.StatusCode}.", ct);
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    /// <summary>
    /// Calls GET /repos/{owner}/{repo}/check-runs?head_sha=&amp;app_id=&amp;filter=latest
    /// to find an already-posted check run for this row's external_id.
    /// Returns <see cref="CheckRunPostResult"/> with <c>AlreadyPosted</c> if found; null if not found.
    /// Refs docs/SPECIFICATION.md:8529-8533.
    /// </summary>
    private async Task<CheckRunPostResult?> TryFindExistingCheckRunAsync(
        PrCheck row, GithubInstallation install, string conclusion, CancellationToken ct)
    {
        string installToken;
        try
        {
            installToken = await tokens.GetOrRefreshAsync(install.InstallationId, ct);
        }
        catch (Exception ex)
        {
            log.LogWarning(ex, "Could not fetch token for idempotency GET for PrCheck {PrCheckId}; proceeding to retry POST", row.Id);
            return null; // fall through to normal POST path
        }

        var (owner, repoName) = SplitRepo(row.Repo);
        var appId = keyProvider.GetAppId();
        var http = httpFactory.CreateClient("github-checks");

        using var req = new HttpRequestMessage(
            HttpMethod.Get,
            $"/repos/{owner}/{repoName}/check-runs?head_sha={row.HeadSha}&app_id={appId}&filter=latest");
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", installToken);
        req.Headers.Accept.ParseAdd("application/vnd.github+json");
        req.Headers.Add("X-GitHub-Api-Version", "2022-11-28");

        HttpResponseMessage rsp;
        try
        {
            rsp = await http.SendAsync(req, ct);
        }
        catch (Exception ex)
        {
            log.LogWarning(ex, "Network error during idempotency GET for PrCheck {PrCheckId}; proceeding to retry POST", row.Id);
            return null;
        }

        using var __ = rsp;

        if (!rsp.IsSuccessStatusCode)
        {
            log.LogWarning("GitHub returned {StatusCode} for idempotency GET for PrCheck {PrCheckId}; proceeding to retry POST",
                (int)rsp.StatusCode, row.Id);
            return null;
        }

        try
        {
            var body = await rsp.Content.ReadAsStringAsync(ct);
            using var doc = JsonDocument.Parse(body);
            var externalIdStr = row.ExternalId.ToString();

            foreach (var run in doc.RootElement.GetProperty("check_runs").EnumerateArray())
            {
                if (run.TryGetProperty("external_id", out var extId) &&
                    string.Equals(extId.GetString(), externalIdStr, StringComparison.OrdinalIgnoreCase))
                {
                    var foundId = run.TryGetProperty("id", out var idEl) ? idEl.GetInt64() : (long?)null;
                    log.LogInformation("Idempotency GET found existing check run {CheckRunId} for PrCheck {PrCheckId}",
                        foundId, row.Id);

                    row.CheckRunId = foundId;
                    row.PostedAt = clock.GetUtcNow().UtcDateTime;
                    row.Status = "posted";
                    row.Conclusion = conclusion;
                    await db.SaveChangesAsync(ct);

                    return new CheckRunPostResult(PrCheckPostStatus.AlreadyPosted, foundId, null, null);
                }
            }
        }
        catch (Exception ex)
        {
            log.LogWarning(ex, "Could not parse idempotency GET response for PrCheck {PrCheckId}; proceeding to retry POST", row.Id);
        }

        return null; // not found — proceed to POST
    }

    private static bool IsRepoCovered(string repo, string repoSelection, string repoSetJson)
    {
        if (string.Equals(repoSelection, "all", StringComparison.OrdinalIgnoreCase))
            return true;

        try
        {
            using var doc = JsonDocument.Parse(repoSetJson);
            foreach (var el in doc.RootElement.EnumerateArray())
            {
                // Repo set entries: {"id":1,"owner":"acme","name":"api"} or {"full_name":"acme/api"}
                string? fullName = null;
                if (el.TryGetProperty("full_name", out var fn))
                    fullName = fn.GetString();
                else if (el.TryGetProperty("owner", out var o) && el.TryGetProperty("name", out var n))
                    fullName = $"{o.GetString()}/{n.GetString()}";

                if (string.Equals(fullName, repo, StringComparison.OrdinalIgnoreCase))
                    return true;
            }
        }
        catch (JsonException)
        {
            // Malformed repo set — fail safe (deny)
        }

        return false;
    }

    private static (string Owner, string RepoName) SplitRepo(string repo)
    {
        var slash = repo.IndexOf('/');
        if (slash < 0) return (repo, repo);
        return (repo[..slash], repo[(slash + 1)..]);
    }

    private string BuildPayload(PrCheck row, string owner, string repoName, string conclusion, DateTime now)
    {
        var obj = new JsonObject
        {
            ["name"] = "ApiTool",
            ["head_sha"] = row.HeadSha,
            ["external_id"] = row.ExternalId.ToString(),
            ["status"] = "completed",
            ["conclusion"] = conclusion,
            ["started_at"] = row.CreatedAt.ToString("O"),
            ["completed_at"] = now.ToString("O"),
        };

        if (!string.IsNullOrEmpty(row.DetailsUrl))
            obj["details_url"] = row.DetailsUrl;

        if (row.OutputSummary is not null || row.OutputText is not null)
        {
            var output = new JsonObject
            {
                ["title"] = "ApiTool Check",
                ["summary"] = row.OutputSummary ?? string.Empty,
            };
            if (row.OutputText is not null)
                output["text"] = row.OutputText;
            obj["output"] = output;
        }

        return obj.ToJsonString();
    }

    private void ObserveRateLimitHeaders(HttpResponseMessage rsp, long installationId)
    {
        int? remaining = null;
        int? retryAfter = null;
        DateTimeOffset? resetAt = null;

        if (rsp.Headers.TryGetValues("X-RateLimit-Remaining", out var rem) &&
            int.TryParse(rem.FirstOrDefault(), out var r))
            remaining = r;

        if (rsp.Headers.TryGetValues("X-RateLimit-Reset", out var reset) &&
            long.TryParse(reset.FirstOrDefault(), out var epoch))
            resetAt = DateTimeOffset.FromUnixTimeSeconds(epoch);

        if (rsp.Headers.TryGetValues("Retry-After", out var ra) &&
            int.TryParse(ra.FirstOrDefault(), out var raVal))
            retryAfter = raVal;

        if (remaining.HasValue || retryAfter.HasValue)
            rateLimit.Update(installationId, remaining, retryAfter, resetAt);
    }

    private async Task<CheckRunPostResult> RecordTransientFailure(PrCheck row, string message, CancellationToken ct)
    {
        const int maxAttempts = 5;
        row.LastError = message;
        var isPermanent = row.AttemptCount >= maxAttempts;
        row.Status = isPermanent ? "failed" : "queued";
        await db.SaveChangesAsync(ct);
        if (isPermanent)
            return new CheckRunPostResult(PrCheckPostStatus.Failed, null, message, PrCheckErrorCode.PermanentFailure);
        return new CheckRunPostResult(PrCheckPostStatus.Queued, null, message, PrCheckErrorCode.GithubUnavailable);
    }

    private static CheckRunPostResult Failed(PrCheckErrorCode code, string message)
        => new(PrCheckPostStatus.Failed, null, message, code);

    private static string ScrubTokens(string? input)
    {
        if (string.IsNullOrEmpty(input)) return string.Empty;
        return ScrubPattern.Replace(input, "[REDACTED]");
    }
}
