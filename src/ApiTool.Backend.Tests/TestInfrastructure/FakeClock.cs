namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// Deterministic <see cref="TimeProvider"/> implementation for tests.
/// Returns a fixed <see cref="DateTimeOffset"/> supplied at construction time.
/// </summary>
public sealed class FakeClock(DateTimeOffset utcNow) : TimeProvider
{
    private DateTimeOffset _utcNow = utcNow;

    /// <inheritdoc/>
    public override DateTimeOffset GetUtcNow() => _utcNow;

    /// <summary>Advances the clock by the given amount.</summary>
    public void Advance(TimeSpan amount) => _utcNow = _utcNow.Add(amount);
}
