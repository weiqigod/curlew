using ApiTool.Backend.Licensing.Trials;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// Test double for <see cref="ITrialStateResolver"/>. Returns a configurable
/// <see cref="TrialStateResult"/> and records the last tier passed to <see cref="ResolveAsync"/>.
/// </summary>
internal sealed class FakeTrialStateResolver : ITrialStateResolver
{
    /// <summary>The result to return from <see cref="ResolveAsync"/>.</summary>
    public TrialStateResult Result { get; set; } = new("none", null, Array.Empty<string>());

    /// <summary>The last tier argument passed to <see cref="ResolveAsync"/>.</summary>
    public string? LastTier { get; private set; }

    /// <inheritdoc/>
    public Task<TrialStateResult> ResolveAsync(Guid userId, string tier, CancellationToken ct = default)
    {
        LastTier = tier;
        return Task.FromResult(Result);
    }
}
