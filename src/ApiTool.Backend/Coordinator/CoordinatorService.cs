using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Results;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Coordinator;

/// <summary>
/// Business logic for creating, claiming, and completing distributed execution jobs.
/// Each job is split into N shards; worker agents claim shards, submit results, and
/// send heartbeats. Stale shards are reclaimed by <see cref="ShardReaper"/>.
/// </summary>
public sealed class CoordinatorService(
    AppDbContext db,
    TimeProvider clock,
    ResultsService results,
    RoleResolver roleResolver,
    ILogger<CoordinatorService> logger) : IShardReaper
{
    /// <summary>Maximum number of shards per job.</summary>
    public const int MaxShardCount = 64;

    /// <summary>Shard heartbeat timeout — coordinator shards not seen within this window are reclaimed.</summary>
    public static readonly TimeSpan HeartbeatTimeout = TimeSpan.FromSeconds(60);

    /// <summary>Heartbeat timeout for scheduled_runs — worker-pull runs not seen within this window are reclaimed.</summary>
    public static readonly TimeSpan ScheduledRunHeartbeatTimeout = TimeSpan.FromMinutes(5);

    // ── CreateJob ─────────────────────────────────────────────────────────────

    /// <summary>
    /// Creates a new coordinator job with <paramref name="request"/>'s shard count.
    /// </summary>
    public async Task<(CoordinatorJobDto? dto, CoordinatorError error, string? message)>
        CreateJobAsync(Guid userId, Guid orgId, CreateJobRequest? request, CancellationToken ct)
    {
        if (!await HasCoordinatorPermissionAsync(userId, orgId, ct))
            return (null, CoordinatorError.PermissionDenied, "Permission denied.");

        if (request is null)
            return (null, CoordinatorError.InvalidRequest, "Request body is required.");

        if (string.IsNullOrWhiteSpace(request.CollectionSha))
            return (null, CoordinatorError.InvalidRequest, "collection_sha is required.");

        var shardCount = request.ShardCount ?? 0;
        if (shardCount < 1 || shardCount > MaxShardCount)
            return (null, CoordinatorError.InvalidShardCount,
                $"shard_count must be between 1 and {MaxShardCount}.");

        var now = clock.GetUtcNow().UtcDateTime;
        var jobId = Guid.NewGuid();

        var job = new CoordinatorJob
        {
            Id = jobId,
            OrgId = orgId,
            CreatedBy = userId,
            CollectionSha = request.CollectionSha,
            ShardCount = shardCount,
            Status = CoordinatorJobStatus.Pending,
            CreatedAt = now,
            UpdatedAt = now,
        };
        db.CoordinatorJobs.Add(job);

        var shards = new List<CoordinatorShard>(shardCount);
        for (var i = 0; i < shardCount; i++)
        {
            var shard = new CoordinatorShard
            {
                Id = Guid.NewGuid(),
                JobId = jobId,
                ShardIndex = i,
                Status = CoordinatorShardStatus.Pending,
                RequestsJson = "[]",
            };
            db.CoordinatorShards.Add(shard);
            shards.Add(shard);
        }

        await db.SaveChangesAsync(ct);
        return (ToJobDto(job, shards), CoordinatorError.None, null);
    }

    // ── ClaimAsync ────────────────────────────────────────────────────────────

    /// <summary>
    /// Claims the oldest pending shard in the job for the given worker.
    /// Returns <see cref="CoordinatorError.NoShardsAvailable"/> when all shards are claimed.
    /// </summary>
    public async Task<(CoordinatorShardDto? dto, CoordinatorError error)>
        ClaimAsync(Guid userId, Guid orgId, Guid jobId, ClaimRequest? request, CancellationToken ct)
    {
        if (!await HasCoordinatorPermissionAsync(userId, orgId, ct))
            return (null, CoordinatorError.PermissionDenied);

        if (string.IsNullOrWhiteSpace(request?.WorkerId))
            return (null, CoordinatorError.InvalidRequest);

        var job = await db.CoordinatorJobs
            .FirstOrDefaultAsync(j => j.Id == jobId && j.OrgId == orgId, ct);
        if (job is null)
            return (null, CoordinatorError.NotFound);

        var now = clock.GetUtcNow().UtcDateTime;

        // Load the lowest-index pending shard (greedy, first-claim-wins).
        var shard = await db.CoordinatorShards
            .Where(s => s.JobId == jobId && s.Status == CoordinatorShardStatus.Pending)
            .OrderBy(s => s.ShardIndex)
            .FirstOrDefaultAsync(ct);

        if (shard is null)
            return (null, CoordinatorError.NoShardsAvailable);

        shard.Status = CoordinatorShardStatus.Running;
        shard.AssignedWorker = request.WorkerId;
        shard.ClaimedAt = now;
        shard.LastHeartbeatAt = now;

        // Transition job to Running on first claim.
        if (job.Status == CoordinatorJobStatus.Pending)
        {
            job.Status = CoordinatorJobStatus.Running;
            job.UpdatedAt = now;
        }

        await db.SaveChangesAsync(ct);
        return (ToShardDto(shard), CoordinatorError.None);
    }

    // ── SubmitResultAsync ─────────────────────────────────────────────────────

    /// <summary>
    /// Submits the outcome of a completed shard. If this is the last shard, aggregates
    /// results via <see cref="ResultsService"/> and marks the job completed.
    /// </summary>
    public async Task<(CoordinatorError error, string? message)>
        SubmitResultAsync(Guid userId, Guid orgId, Guid jobId, Guid shardId,
            SubmitResultRequest? request, CancellationToken ct)
    {
        if (!await HasCoordinatorPermissionAsync(userId, orgId, ct))
            return (CoordinatorError.PermissionDenied, "Permission denied.");

        if (string.IsNullOrWhiteSpace(request?.WorkerId))
            return (CoordinatorError.InvalidRequest, "worker_id is required.");

        var job = await db.CoordinatorJobs
            .FirstOrDefaultAsync(j => j.Id == jobId && j.OrgId == orgId, ct);
        if (job is null)
            return (CoordinatorError.NotFound, "Job not found.");

        var shard = await db.CoordinatorShards
            .FirstOrDefaultAsync(s => s.Id == shardId && s.JobId == jobId, ct);
        if (shard is null)
            return (CoordinatorError.NotFound, "Shard not found.");

        if (shard.AssignedWorker != request.WorkerId)
            return (CoordinatorError.ShardNotClaimed, "Shard is not owned by the calling worker.");

        if (shard.Status != CoordinatorShardStatus.Running)
            return (CoordinatorError.InvalidState, "Shard is not in the Running state.");

        var now = clock.GetUtcNow().UtcDateTime;
        shard.Status = CoordinatorShardStatus.Completed;
        shard.CompletedAt = now;
        shard.PassCount = request.PassCount;
        shard.FailCount = request.FailCount;
        shard.DurationMs = request.DurationMs;

        if (request.Items is { Count: > 0 })
            shard.ResultJson = JsonSerializer.Serialize(request.Items);

        // Check if this is the last shard to complete.
        var remainingShards = await db.CoordinatorShards
            .CountAsync(s => s.JobId == jobId && s.Status != CoordinatorShardStatus.Completed, ct);

        // The current shard is still Running in the DB (not yet saved), so remaining=1 means
        // this is the last shard. Use == 1 for precision to avoid double-aggregation on retries.
        if (remainingShards == 1)
        {
            var aggregateError = await AggregateAndWriteResultAsync(job, shard, now, ct);
            if (aggregateError != CoordinatorError.None)
                return (aggregateError, "Failed to aggregate results.");
        }

        job.UpdatedAt = now;
        await db.SaveChangesAsync(ct);
        return (CoordinatorError.None, null);
    }

    // ── HeartbeatAsync ────────────────────────────────────────────────────────

    /// <summary>
    /// Updates <see cref="CoordinatorShard.LastHeartbeatAt"/> to keep the shard alive.
    /// </summary>
    public async Task<(CoordinatorError error, string? message)>
        HeartbeatAsync(Guid userId, Guid orgId, Guid jobId, Guid shardId,
            HeartbeatRequest? request, CancellationToken ct)
    {
        if (!await HasCoordinatorPermissionAsync(userId, orgId, ct))
            return (CoordinatorError.PermissionDenied, "Permission denied.");

        var shard = await db.CoordinatorShards
            .FirstOrDefaultAsync(s => s.Id == shardId && s.JobId == jobId, ct);
        if (shard is null)
            return (CoordinatorError.NotFound, "Shard not found.");

        if (shard.Status != CoordinatorShardStatus.Running)
            return (CoordinatorError.InvalidState, "Shard is not in the Running state.");

        if (shard.AssignedWorker != request?.WorkerId)
            return (CoordinatorError.ShardNotClaimed, "Shard is not owned by the calling worker.");

        shard.LastHeartbeatAt = clock.GetUtcNow().UtcDateTime;
        await db.SaveChangesAsync(ct);
        return (CoordinatorError.None, null);
    }

    // ── GetJobAsync ───────────────────────────────────────────────────────────

    /// <summary>Returns the current state of a job including all its shards.</summary>
    public async Task<(CoordinatorJobDto? dto, CoordinatorError error)>
        GetJobAsync(Guid userId, Guid orgId, Guid jobId, CancellationToken ct)
    {
        if (!await HasCoordinatorPermissionAsync(userId, orgId, ct))
            return (null, CoordinatorError.PermissionDenied);

        var job = await db.CoordinatorJobs
            .FirstOrDefaultAsync(j => j.Id == jobId && j.OrgId == orgId, ct);
        if (job is null)
            return (null, CoordinatorError.NotFound);

        var shards = await db.CoordinatorShards
            .Where(s => s.JobId == jobId)
            .OrderBy(s => s.ShardIndex)
            .ToListAsync(ct);

        return (ToJobDto(job, shards), CoordinatorError.None);
    }

    // ── IShardReaper ──────────────────────────────────────────────────────────

    /// <summary>
    /// Reclaims coordinator shards whose last heartbeat is older than <see cref="HeartbeatTimeout"/>
    /// and scheduled_runs whose last heartbeat is older than <see cref="ScheduledRunHeartbeatTimeout"/>.
    /// Returns the total number of items reclaimed across both domains.
    /// </summary>
    public async Task<int> ReapStaleShardsAsync(CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        var shardCutoff = now - HeartbeatTimeout;
        var runCutoff = now - ScheduledRunHeartbeatTimeout;

        var staleShards = await db.CoordinatorShards
            .Where(s => s.Status == CoordinatorShardStatus.Running
                     && s.LastHeartbeatAt < shardCutoff)
            .ToListAsync(ct);

        foreach (var shard in staleShards)
        {
            shard.Status = CoordinatorShardStatus.Pending;
            shard.AssignedWorker = null;
            shard.ClaimedAt = null;
            shard.LastHeartbeatAt = null;
        }

        // M16-009: also reap stale scheduled_runs back to queued.
        var staleRuns = await db.ScheduledRuns
            .Where(r => r.Status == ScheduledRunStatus.Running
                     && r.LastHeartbeatAt != null
                     && r.LastHeartbeatAt < runCutoff)
            .ToListAsync(ct);

        foreach (var run in staleRuns)
        {
            run.Status = ScheduledRunStatus.Queued;
            run.ClaimToken = null;
            run.ClaimedByWorker = null;
            run.ClaimedAt = null;
            run.LastHeartbeatAt = null;
            run.ClaimDeadline = null;
            run.StartedAt = null;
        }

        if (staleShards.Count == 0 && staleRuns.Count == 0)
            return 0;

        await db.SaveChangesAsync(ct);
        return staleShards.Count + staleRuns.Count;
    }

    // ── private helpers ───────────────────────────────────────────────────────

    private async Task<CoordinatorError> AggregateAndWriteResultAsync(
        CoordinatorJob job, CoordinatorShard lastShard, DateTime now, CancellationToken ct)
    {
        // Load all shards (including the one we're about to complete) for aggregation.
        var allShards = await db.CoordinatorShards
            .Where(s => s.JobId == job.Id && s.Id != lastShard.Id)
            .ToListAsync(ct);
        allShards.Add(lastShard);

        var totalPass = allShards.Sum(s => s.PassCount);
        var totalFail = allShards.Sum(s => s.FailCount);
        var totalDuration = allShards.Sum(s => s.DurationMs);

        // Flatten per-shard result items.
        var items = new List<Results.UploadResultItemRequest>();
        foreach (var shard in allShards)
        {
            if (string.IsNullOrWhiteSpace(shard.ResultJson))
                continue;

            try
            {
                var shardItems = JsonSerializer.Deserialize<List<JsonElement>>(shard.ResultJson);
                if (shardItems is not null)
                {
                    foreach (var item in shardItems)
                    {
                        var name = item.TryGetProperty("name", out var n) ? n.GetString() : null;
                        var status = item.TryGetProperty("status", out var s) ? s.GetString() : null;
                        var duration = item.TryGetProperty("duration_ms", out var d) ? d.GetInt64() : 0L;
                        var message = item.TryGetProperty("message", out var m) ? m.GetString() : null;
                        if (!string.IsNullOrEmpty(name) && !string.IsNullOrEmpty(status))
                            items.Add(new Results.UploadResultItemRequest(name, status, duration, message));
                    }
                }
            }
            catch (JsonException)
            {
                // Ignore malformed shard results; aggregate what we can.
            }
        }

        var uploadRequest = new Results.UploadResultRequest(
            CollectionName: job.CollectionSha,
            RunAt: now,
            DurationMs: totalDuration,
            PassCount: totalPass,
            FailCount: totalFail,
            SkippedCount: 0,
            TriggeredBy: "coordinator",
            GitSha: null,
            Items: items);

        // CreatedBy is Guid? (nullable after M18-006 anonymisation widening); use Empty as
        // a system fallback when the original creator has been anonymised.
        var (resultDto, resultError, resultMessage, _) = await results.IngestAsync(
            job.CreatedBy ?? Guid.Empty, job.OrgId, uploadRequest, ct);

        if (resultError != Results.ResultError.None)
        {
            logger.LogError(
                "Coordinator job {JobId} aggregation failed: IngestAsync returned {Error} — {Message}",
                CoordinatorJobId.Format(job.Id), resultError, resultMessage);
            return resultError == Results.ResultError.PermissionDenied
                ? CoordinatorError.PermissionDenied
                : CoordinatorError.InvalidRequest;
        }

        if (resultDto is not null && Results.ResultId.TryParse(resultDto.Id, out var aggregateId))
            job.AggregateResultId = aggregateId;

        job.Status = CoordinatorJobStatus.Completed;
        job.CompletedAt = now;
        return CoordinatorError.None;
    }

    private Task<bool> HasCoordinatorPermissionAsync(Guid userId, Guid orgId, CancellationToken ct)
        => roleResolver.HasPermissionAsync(userId, orgId, Permissions.CoordinatorWorker, ct);

    private static CoordinatorJobDto ToJobDto(CoordinatorJob job, IReadOnlyList<CoordinatorShard> shards) =>
        new(
            JobId: CoordinatorJobId.Format(job.Id),
            OrgId: job.OrgId,
            CollectionSha: job.CollectionSha,
            ShardCount: job.ShardCount,
            State: job.Status.ToString().ToLowerInvariant(),
            CreatedAt: job.CreatedAt,
            CompletedAt: job.CompletedAt,
            AggregateResultId: job.AggregateResultId.HasValue
                ? Results.ResultId.Format(job.AggregateResultId.Value) : null,
            Shards: shards.Select(ToShardDto).ToList());

    private static CoordinatorShardDto ToShardDto(CoordinatorShard shard) =>
        new(
            ShardId: ShardId.Format(shard.Id),
            JobId: CoordinatorJobId.Format(shard.JobId),
            ShardIndex: shard.ShardIndex,
            State: shard.Status.ToString().ToLowerInvariant(),
            AssignedWorker: shard.AssignedWorker,
            RequestsJson: shard.RequestsJson);
}
