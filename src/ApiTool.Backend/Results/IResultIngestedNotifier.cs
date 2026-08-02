namespace ApiTool.Backend.Results;

/// <summary>
/// Seam for downstream dispatchers (e.g. M4-008 notifications) to react to a newly
/// ingested result. The default <see cref="NoopResultIngestedNotifier"/> is a no-op.
/// </summary>
public interface IResultIngestedNotifier
{
    /// <summary>
    /// Called after a result has been persisted to the database.
    /// </summary>
    /// <param name="orgId">The organization that owns the result.</param>
    /// <param name="resultId">The newly persisted result's id.</param>
    /// <param name="ct">Cancellation token.</param>
    Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct);
}
