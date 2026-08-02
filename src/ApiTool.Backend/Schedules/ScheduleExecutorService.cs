using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using ApiTool.Backend.Schedules.Keys;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Schedules;

/// <summary>
/// Business logic for the schedule executor worker-pull protocol: claim, heartbeat, and result submission.
/// Workers call GET /api/v1/schedules/next-run to atomically claim the oldest queued run, then send
/// heartbeats, and finally submit their result. The <see cref="ClaimNextAsync"/> method is the entry
/// point; the run's status transitions Queued → Running → Completed|Failed.
/// <para>
/// Race-safety note: EF SaveChanges default behaviour is "last write wins" — we do not use a rowversion
/// column in this slice. Concurrent claims are benign in the common case: between the two calls the row
/// is already Running so the second caller's next-run query (filtering by Status == Queued) returns null
/// and gets 204. The retry-on-zero-affected-rows hardening is deferred to a future task.
/// </para>
/// </summary>
public sealed class ScheduleExecutorService(
    AppDbContext db,
    TimeProvider clock,
    ResultsService results,
    IScheduleEnvKeyProvider envKeyProvider,
    ILogger<ScheduleExecutorService> logger)
{
    /// <summary>Max time a claim is considered fresh before the reaper considers it stale.</summary>
    public static readonly TimeSpan ClaimDeadlineDuration = TimeSpan.FromSeconds(30);

    // ── ClaimNextAsync ────────────────────────────────────────────────────────

    /// <summary>
    /// Atomically claims the oldest queued scheduled_run for the org. Returns
    /// <see cref="ScheduleClaimError.NoRunsAvailable"/> when there is none.
    /// </summary>
    /// <param name="orgId">The org derived from the worker's JWT.</param>
    /// <param name="workerId">Opaque worker identifier sourced from JWT claim or X-Worker-Id header.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(NextRunResponse? dto, ScheduleClaimError error)>
        ClaimNextAsync(Guid orgId, string? workerId, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        var deadline = now + ClaimDeadlineDuration;

        // Find the oldest queued run for any schedule belonging to the org.
        var candidate = await db.ScheduledRuns
            .Where(r => r.Status == ScheduledRunStatus.Queued
                     && db.Schedules.Any(s => s.Id == r.ScheduleId && s.OrgId == orgId))
            .OrderBy(r => r.CreatedAt)
            .FirstOrDefaultAsync(ct);

        if (candidate is null)
            return (null, ScheduleClaimError.NoRunsAvailable);

        var schedule = await db.Schedules.SingleAsync(s => s.Id == candidate.ScheduleId, ct);

        candidate.Status = ScheduledRunStatus.Running;
        candidate.ClaimToken = Guid.NewGuid();
        candidate.ClaimedByWorker = workerId ?? "unknown";
        candidate.ClaimedAt = now;
        candidate.StartedAt = now;
        candidate.LastHeartbeatAt = now;
        candidate.ClaimDeadline = deadline;

        await db.SaveChangesAsync(ct);

        // Decrypt env_vars if present (M18-009). Null ciphertext → empty dict (no env vars set).
        // A decrypt failure means the ciphertext is corrupted or the KEK was rotated without
        // re-encryption — propagate as DecryptionFailed so the run is not claimed with a wrong
        // env-var set. The run stays in Running state; the worker should not retry the claim.
        Dictionary<string, string> envVars;
        if (schedule.EnvVarsCiphertext is { Length: > 0 } ciphertext && schedule.EnvVarsKid is { } kid)
        {
            try
            {
                var envJson = await envKeyProvider.DecryptAsync(ciphertext, kid, ct);
                envVars = JsonSerializer.Deserialize<Dictionary<string, string>>(envJson,
                    new JsonSerializerOptions { PropertyNameCaseInsensitive = true })
                    ?? new Dictionary<string, string>(StringComparer.Ordinal);
            }
            catch (ScheduleEnvDecryptException ex)
            {
                logger.LogError(ex,
                    "Failed to decrypt env_vars for schedule {ScheduleId} (kid={Kid}) — returning DecryptionFailed",
                    ScheduleId.Format(candidate.ScheduleId), kid);
                return (null, ScheduleClaimError.DecryptionFailed);
            }
        }
        else
        {
            envVars = new Dictionary<string, string>(StringComparer.Ordinal);
        }

        return (new NextRunResponse(
            RunId: RunId.Format(candidate.Id),
            ScheduleId: ScheduleId.Format(candidate.ScheduleId),
            CollectionRef: schedule.CollectionRef,
            EnvVars: envVars,
            ClaimToken: candidate.ClaimToken.Value.ToString(),
            Deadline: deadline), ScheduleClaimError.None);
    }

    // ── HeartbeatAsync ────────────────────────────────────────────────────────

    /// <summary>
    /// Updates last_heartbeat_at if the run is still claimed by this token. Returns
    /// <see cref="ScheduleClaimError.ClaimReaped"/> when the row was reaped (status != Running
    /// or claim_token cleared).
    /// </summary>
    public async Task<ScheduleClaimError> HeartbeatAsync(
        Guid orgId, Guid runId, Guid claimToken, CancellationToken ct)
    {
        var run = await db.ScheduledRuns
            .Where(r => r.Id == runId
                     && db.Schedules.Any(s => s.Id == r.ScheduleId && s.OrgId == orgId))
            .SingleOrDefaultAsync(ct);

        if (run is null)
            return ScheduleClaimError.NotFound;

        // Reap window: row was sent back to Queued by reaper → claim_token cleared.
        if (run.Status != ScheduledRunStatus.Running || run.ClaimToken is null)
            return ScheduleClaimError.ClaimReaped;

        if (run.ClaimToken != claimToken)
            return ScheduleClaimError.StaleClaim;

        run.LastHeartbeatAt = clock.GetUtcNow().UtcDateTime;
        await db.SaveChangesAsync(ct);
        return ScheduleClaimError.None;
    }

    // ── SubmitResultAsync ─────────────────────────────────────────────────────

    /// <summary>
    /// Persists the result and transitions the run to Completed or Failed.
    /// Reuses <see cref="ResultsService.IngestAsync"/> for the results write.
    /// </summary>
    public async Task<ScheduleClaimError> SubmitResultAsync(
        Guid userId, Guid orgId, Guid runId, ScheduleResultRequest request, CancellationToken ct)
    {
        if (string.IsNullOrWhiteSpace(request.ClaimToken)
            || !Guid.TryParse(request.ClaimToken, out var token))
            return ScheduleClaimError.InvalidRequest;

        var run = await db.ScheduledRuns
            .Where(r => r.Id == runId
                     && db.Schedules.Any(s => s.Id == r.ScheduleId && s.OrgId == orgId))
            .SingleOrDefaultAsync(ct);

        if (run is null)
            return ScheduleClaimError.NotFound;

        // Idempotency: same worker posting the same claim_token after a successful first post is a
        // benign network retry — return None (200) without re-inserting the result row.
        if ((run.Status == ScheduledRunStatus.Completed || run.Status == ScheduledRunStatus.Failed)
            && run.ClaimToken == token
            && run.ResultId is not null)
        {
            logger.LogInformation(
                "Schedule executor result idempotent retry for run {RunId} (same claim_token, result_id={ResultId}).",
                RunId.Format(runId), run.ResultId);
            return ScheduleClaimError.None;
        }

        if (run.Status == ScheduledRunStatus.Completed || run.Status == ScheduledRunStatus.Failed)
            return ScheduleClaimError.AlreadyCompleted;

        // Reap window: claim_token cleared means the reaper already reset this row.
        if (run.Status != ScheduledRunStatus.Running || run.ClaimToken is null)
            return ScheduleClaimError.ClaimReaped;

        if (run.ClaimToken != token)
            return ScheduleClaimError.StaleClaim;

        var upload = new UploadResultRequest(
            CollectionName: request.CollectionName,
            RunAt: request.RunAt,
            DurationMs: request.DurationMs,
            PassCount: request.PassCount,
            FailCount: request.FailCount,
            SkippedCount: request.SkippedCount,
            TriggeredBy: request.TriggeredBy ?? "schedule",
            GitSha: request.GitSha,
            Items: request.Items);

        var (resultDto, resultError, message, _) = await results.IngestAsync(userId, orgId, upload, ct);
        if (resultError != ResultError.None)
        {
            logger.LogError(
                "Schedule executor result ingest failed for run {RunId}: {Error} {Message}",
                RunId.Format(runId), resultError, message);
            return resultError == ResultError.PermissionDenied
                ? ScheduleClaimError.NotFound      // no info leak
                : ScheduleClaimError.InvalidRequest;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        run.Status = (request.FailCount ?? 0) == 0
            ? ScheduledRunStatus.Completed
            : ScheduledRunStatus.Failed;
        run.CompletedAt = now;

        if (resultDto is not null && ResultId.TryParse(resultDto.Id, out var resultGuid))
            run.ResultId = resultGuid;

        if (run.Status == ScheduledRunStatus.Failed)
            run.FailureReason = $"fail_count={request.FailCount}";

        await db.SaveChangesAsync(ct);
        return ScheduleClaimError.None;
    }
}
