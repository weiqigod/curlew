// Refs docs/SPECIFICATION.md:8565 (check_run.rerequested → re-run pipeline).
namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Seam for enqueuing a collection re-run job triggered by a GitHub
/// <c>check_run.rerequested</c> event. The concrete implementation lives in the
/// re-run pipeline (out of M14 scope); the production default is a structured-log
/// stub. Swap for a real queue implementation once the pipeline exists.
/// </summary>
public interface IRerunJobQueue
{
    /// <summary>
    /// Enqueue a re-run for the ApiTool collection that produced the given
    /// <paramref name="externalId"/> check-run at the specified <paramref name="headSha"/>.
    /// </summary>
    Task EnqueueAsync(Guid externalId, string headSha, CancellationToken ct);
}
