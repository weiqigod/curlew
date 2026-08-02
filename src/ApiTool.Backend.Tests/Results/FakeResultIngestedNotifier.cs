using ApiTool.Backend.Results;

namespace ApiTool.Backend.Tests.Results;

/// <summary>
/// In-memory test double for <see cref="IResultIngestedNotifier"/> that records calls.
/// </summary>
internal sealed class FakeResultIngestedNotifier : IResultIngestedNotifier
{
    private readonly List<(Guid orgId, Guid resultId)> _calls = new();

    /// <summary>All recorded notification calls.</summary>
    public IReadOnlyList<(Guid orgId, Guid resultId)> Calls => _calls;

    /// <inheritdoc/>
    public Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct)
    {
        _calls.Add((orgId, resultId));
        return Task.CompletedTask;
    }
}
