// Refs docs/SPECIFICATION.md:8565 (check_run.rerequested → re-run pipeline).
// Production stub — emits a structured log so the signal is never silently dropped.
// Replace with a real queue implementation once the re-run pipeline exists (post-M14).
namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Default <see cref="IRerunJobQueue"/> implementation that logs the enqueue request.
/// Used until the re-run pipeline is built. Spec :8565.
/// </summary>
public sealed class LoggingRerunJobQueue(ILogger<LoggingRerunJobQueue> log) : IRerunJobQueue
{
    /// <inheritdoc/>
    public Task EnqueueAsync(Guid externalId, string headSha, CancellationToken ct)
    {
        log.LogInformation(
            "github_rerun_job_enqueued external_id={ExternalId} head_sha={HeadSha}",
            externalId, headSha);
        return Task.CompletedTask;
    }
}
