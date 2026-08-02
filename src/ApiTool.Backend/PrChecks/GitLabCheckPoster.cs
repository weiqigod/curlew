// Refs docs/SPECIFICATION.md:9216-9246 (outbound GitLab Commit Status API POST).
using System.Net;
using System.Text.Json;
using System.Text.Json.Nodes;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Posts commit statuses to GitLab's Commit Status API on behalf of an organisation's
/// <c>gitlab_installations</c> row. Uses PAT-based auth (decrypted per-request via
/// <see cref="IGitLabKeyProvider"/>), per-installation rate-limit tracking, HTTPS URL
/// enforcement, and lossy state mapping.
/// Refs docs/SPECIFICATION.md:9216-9246.
/// </summary>
public sealed class GitLabCheckPoster(
    AppDbContext db,
    IGitLabKeyProvider keyProvider,
    IGitLabRateLimitTracker rateLimit,
    IHttpClientFactory httpFactory,
    IOptions<GitLabOptions> options,
    IAuditWriter audit,
    TimeProvider clock,
    ILogger<GitLabCheckPoster> log) : IGitLabCheckPoster
{
    private static readonly JsonSerializerOptions JsonOpts = new(JsonSerializerDefaults.Web);

    /// <inheritdoc/>
    public async Task<CheckRunPostResult> PostAsync(Guid prCheckId, CancellationToken ct)
    {
        // Step 1: Load the PrCheck row
        var row = await db.PrChecks.FindAsync([prCheckId], ct)
            ?? throw new ArgumentException($"PrCheck {prCheckId} not found", nameof(prCheckId));

        // Step 2: Resolve the GitLab installation
        if (row.GitLabInstallationId is null)
        {
            log.LogWarning("PrCheck {Id} has no GitLabInstallationId set", prCheckId);
            return Failed(PrCheckErrorCode.GitLabNoInstallation, "No GitLab installation linked to this PR check.");
        }

        var installation = await db.GitLabInstallations.FindAsync([row.GitLabInstallationId.Value], ct);
        if (installation is null || installation.DeletedAt.HasValue)
        {
            log.LogWarning("GitLab installation {Id} not found or deleted for PrCheck {PrCheckId}",
                row.GitLabInstallationId.Value, prCheckId);
            return Failed(PrCheckErrorCode.GitLabNoInstallation, "GitLab installation not found or deleted.");
        }

        // Step 3: Short-circuit if PAT is already known to be revoked
        if (installation.AccessTokenRevokedAt.HasValue)
        {
            log.LogWarning("GitLab installation {Id} PAT already revoked at {RevokedAt}",
                installation.Id, installation.AccessTokenRevokedAt.Value);
            await MarkFailed(row, "gitlab_token_revoked", "GitLab PAT was previously revoked.", ct);
            return Failed(PrCheckErrorCode.GitLabTokenRevoked, "GitLab PAT has been revoked.");
        }

        // Step 4: HTTPS enforcement (Open Decision 2)
        var baseUrl = installation.GitLabBaseUrl.TrimEnd('/');
        if (baseUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase)
            && !options.Value.Poster.AllowHttp)
        {
            log.LogError("GitLab base URL '{BaseUrl}' is HTTP and GITLAB__ALLOW_HTTP is not set",
                installation.GitLabBaseUrl);
            await MarkFailed(row, "failed",
                $"Insecure GitLab base URL '{installation.GitLabBaseUrl}'. Set GITLAB__ALLOW_HTTP=true to override.", ct);
            return Failed(PrCheckErrorCode.GitLabHttpInsecure,
                $"Insecure GitLab base URL: {installation.GitLabBaseUrl}");
        }

        // Step 5: Rate-limit pre-check
        if (rateLimit.IsBlocked(installation.Id))
        {
            log.LogInformation("GitLab installation {Id} is rate-limited, queuing PrCheck {PrCheckId}",
                installation.Id, prCheckId);
            await MarkStatus(row, "queued", ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null,
                "GitLab rate-limited — queued for retry.", PrCheckErrorCode.GitLabRateLimited);
        }

        // Step 6: Map state + build description
        if (!GitLabStateMapper.TryMap(row.State, out var gitlabState))
        {
            log.LogError("Unknown CLI state '{State}' for PrCheck {Id}", row.State, prCheckId);
            await MarkFailed(row, "failed", $"Unknown state: {row.State}", ct);
            return Failed(PrCheckErrorCode.GitLabPermanentFailure, $"Unknown CLI state: {row.State}");
        }
        var description = GitLabStateMapper.BuildDescription(row.State, row.OutputSummary);

        // Step 7: Decrypt PAT
        byte[] patBytes;
        try
        {
            patBytes = await keyProvider.DecryptAsync(
                installation.AccessTokenCiphertext, installation.AccessTokenKid, ct);
        }
        catch (GitLabPatDecryptException ex)
        {
            log.LogError(ex, "Failed to decrypt GitLab PAT for installation {Id}", installation.Id);
            await MarkStatus(row, "queued", ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null,
                "PAT decryption failed — queued for retry.", PrCheckErrorCode.GitLabUnreachable);
        }
        var pat = System.Text.Encoding.UTF8.GetString(patBytes);

        // Step 8: Record posting started
        var now = clock.GetUtcNow().UtcDateTime;
        row.PostingStartedAt = now;
        row.AttemptCount++;
        row.Status = "posting";
        await db.SaveChangesAsync(ct);

        // Step 9: Build URL and payload
        var url = $"{baseUrl}/api/v4/projects/{installation.ProjectId}/statuses/{row.HeadSha}";

        // Validate target_url: only include if HTTPS
        string? targetUrl = null;
        if (!string.IsNullOrEmpty(row.DetailsUrl))
        {
            if (row.DetailsUrl.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
                targetUrl = row.DetailsUrl;
            else
                log.LogWarning("PrCheck {Id} details_url '{Url}' is not HTTPS — dropping from GitLab payload",
                    prCheckId, row.DetailsUrl);
        }

        var payload = new JsonObject
        {
            ["state"] = gitlabState,
            ["name"] = "ApiTool",
            ["description"] = description,
            ["context"] = "ci/apitool",
        };
        if (targetUrl is not null)
            payload["target_url"] = targetUrl;

        // Step 10: Send HTTP request
        using var requestContent = new System.Net.Http.StringContent(
            payload.ToJsonString(), System.Text.Encoding.UTF8, "application/json");

        using var httpRequest = new HttpRequestMessage(HttpMethod.Post, url)
        {
            Content = requestContent,
        };
        httpRequest.Headers.TryAddWithoutValidation("Private-Token", pat);

        HttpResponseMessage response;
        try
        {
            var client = httpFactory.CreateClient("gitlab-statuses");
            response = await client.SendAsync(httpRequest, ct);
        }
        catch (Exception ex)
        {
            log.LogWarning(ex, "Network error posting to GitLab for PrCheck {Id}", prCheckId);
            await MarkStatus(row, "queued", ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null,
                "Network error — queued for retry.", PrCheckErrorCode.GitLabUnreachable);
        }

        // Step 11: Observe rate-limit headers
        UpdateRateLimitFromHeaders(installation.Id, response);

        // Step 12: Handle response
        var responseBody = await response.Content.ReadAsStringAsync(ct);

        if (response.IsSuccessStatusCode)
        {
            // Parse GitLab status id
            long? statusId = null;
            try
            {
                using var doc = JsonDocument.Parse(responseBody);
                if (doc.RootElement.TryGetProperty("id", out var idProp))
                    statusId = idProp.GetInt64();
            }
            catch (JsonException) { /* non-critical — status id is informational */ }

            row.GitLabStatusId = statusId;
            row.PostedAt = clock.GetUtcNow().UtcDateTime;
            row.Status = "posted";
            row.Conclusion = gitlabState;
            row.LastError = null;
            await db.SaveChangesAsync(ct);

            log.LogInformation("Posted GitLab status {StatusId} for PrCheck {PrCheckId} (state={State})",
                statusId, prCheckId, gitlabState);
            return new CheckRunPostResult(PrCheckPostStatus.Posted, null, null, PrCheckErrorCode.None);
        }

        if (response.StatusCode == HttpStatusCode.Unauthorized)
        {
            // PAT revoked — mark installation + audit
            if (!installation.AccessTokenRevokedAt.HasValue)
                installation.AccessTokenRevokedAt = clock.GetUtcNow().UtcDateTime;

            audit.Append(new AuditEvent(
                OrgId: installation.OrgId,
                ActorId: Guid.Empty,
                EventType: "gitlab.pat.revoked",
                TargetType: "gitlab_installation",
                TargetId: installation.Id,
                Payload: new { installation_id = installation.Id, project_id = installation.ProjectId },
                Success: false,
                FailureReason: "401_from_gitlab"));

            row.Status = "gitlab_token_revoked";
            row.LastError = "GitLab returned 401 — PAT may be revoked.";
            await db.SaveChangesAsync(ct);

            log.LogWarning(
                new EventId(9277, "GITLAB_PAT_REVOKED"),
                "GitLab PAT revoked for installation {InstallationId} " +
                "(org_id={OrgId} project_id={ProjectId} gitlab_base_url={BaseUrl}) " +
                "security_incident=true",
                installation.Id, installation.OrgId, installation.ProjectId, installation.GitLabBaseUrl);

            return Failed(PrCheckErrorCode.GitLabTokenRevoked, "GitLab returned 401 — PAT revoked.");
        }

        if (response.StatusCode == HttpStatusCode.TooManyRequests)
        {
            await MarkStatus(row, "queued", ct);
            return new CheckRunPostResult(PrCheckPostStatus.Queued, null,
                "GitLab rate-limited.", PrCheckErrorCode.GitLabRateLimited);
        }

        if ((int)response.StatusCode is >= 400 and < 500)
        {
            // Client error — permanent failure (bad payload, project not found, etc.)
            row.Status = "failed";
            row.LastError = $"GitLab returned {(int)response.StatusCode}.";
            await db.SaveChangesAsync(ct);
            log.LogError("GitLab permanent failure {Status} for PrCheck {Id}: {Body}",
                (int)response.StatusCode, prCheckId, responseBody[..Math.Min(200, responseBody.Length)]);
            return Failed(PrCheckErrorCode.GitLabPermanentFailure,
                $"GitLab returned {(int)response.StatusCode}.");
        }

        // 5xx transient error — queue for retry
        if (row.AttemptCount >= 5)
        {
            row.Status = "failed";
            row.LastError = $"GitLab returned {(int)response.StatusCode} after {row.AttemptCount} attempts.";
            await db.SaveChangesAsync(ct);
            return Failed(PrCheckErrorCode.GitLabPermanentFailure, "Max retry attempts reached.");
        }

        row.Status = "queued";
        row.LastError = $"GitLab returned {(int)response.StatusCode} — queued for retry.";
        await db.SaveChangesAsync(ct);
        return new CheckRunPostResult(PrCheckPostStatus.Queued, null,
            $"GitLab returned {(int)response.StatusCode} — queued.", PrCheckErrorCode.GitLabUnreachable);
    }

    // ── Private helpers ────────────────────────────────────────────────────────

    private void UpdateRateLimitFromHeaders(Guid installationId, HttpResponseMessage response)
    {
        int? remaining = null;
        int? retryAfter = null;
        DateTimeOffset? resetAt = null;

        if (response.Headers.TryGetValues("RateLimit-Remaining", out var rem)
            && int.TryParse(rem.FirstOrDefault(), out var r))
            remaining = r;

        if (response.Headers.TryGetValues("Retry-After", out var ra)
            && int.TryParse(ra.FirstOrDefault(), out var ras))
            retryAfter = ras;

        if (response.Headers.TryGetValues("RateLimit-Reset", out var reset)
            && long.TryParse(reset.FirstOrDefault(), out var epoch))
            resetAt = DateTimeOffset.FromUnixTimeSeconds(epoch);

        if (remaining.HasValue || retryAfter.HasValue || resetAt.HasValue)
            rateLimit.Update(installationId, remaining, retryAfter, resetAt);
    }

    private async Task MarkStatus(PrCheck row, string status, CancellationToken ct)
    {
        row.Status = status;
        await db.SaveChangesAsync(ct);
    }

    private async Task MarkFailed(PrCheck row, string status, string error, CancellationToken ct)
    {
        row.Status = status;
        row.LastError = error;
        await db.SaveChangesAsync(ct);
    }

    private static CheckRunPostResult Failed(PrCheckErrorCode code, string error) =>
        new(PrCheckPostStatus.Failed, null, error, code);
}
