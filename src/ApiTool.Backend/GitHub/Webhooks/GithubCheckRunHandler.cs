// Refs docs/SPECIFICATION.md:8565 (check_run.rerequested → re-run pipeline).
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Handles <c>check_run.rerequested</c> GitHub webhook events. Spec :8565.
/// Looks up the originating <see cref="Data.Entities.PrCheck"/> row by
/// <c>external_id</c>, then enqueues a re-run job via <see cref="IRerunJobQueue"/>.
/// The production default queue is <see cref="LoggingRerunJobQueue"/> (structured log
/// only) until the re-run pipeline exists; swap in a concrete queue post-M14.
/// </summary>
public class GithubCheckRunHandler(
    AppDbContext db,
    IRerunJobQueue rerunQueue,
    ILogger<GithubCheckRunHandler> log)
{
    /// <summary>Handles a <c>check_run.rerequested</c> payload.</summary>
    public virtual async Task HandleRerequestedAsync(
        GithubCheckRunRerequestedPayload payload, CancellationToken ct)
    {
        if (!Guid.TryParse(payload.ExternalId, out var externalId))
        {
            log.LogWarning(
                "github_check_run_rerequested_unknown_external_id external_id={Raw}",
                payload.ExternalId);
            return;
        }

        var row = await db.PrChecks.FirstOrDefaultAsync(x => x.ExternalId == externalId, ct);
        if (row is null)
        {
            log.LogWarning(
                "github_check_run_rerequested_unknown_pr_check external_id={ExternalId}", externalId);
            return;
        }

        log.LogInformation(
            "github_check_run_rerequested external_id={ExternalId} head_sha={HeadSha} repo={Repo} pr={Pr}",
            externalId, payload.HeadSha, row.Repo, row.Pr);

        await rerunQueue.EnqueueAsync(externalId, payload.HeadSha, ct);
    }
}
