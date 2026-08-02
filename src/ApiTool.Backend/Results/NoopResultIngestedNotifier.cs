namespace ApiTool.Backend.Results;

/// <summary>
/// Default no-op implementation of <see cref="IResultIngestedNotifier"/>.
/// M4-008 replaces this with an event dispatcher.
/// </summary>
public sealed class NoopResultIngestedNotifier : IResultIngestedNotifier
{
    /// <inheritdoc/>
    public Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct) => Task.CompletedTask;
}
