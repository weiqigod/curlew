using System.Collections.Concurrent;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// Per-installation GitHub API rate-limit tracker.
/// Blocks new posts when <c>X-RateLimit-Remaining</c> drops below 100 or
/// when a <c>Retry-After</c> header is received (secondary rate-limit).
/// </summary>
public sealed class RateLimitTracker(TimeProvider clock) : IRateLimitTracker
{
    private const int LowWatermark = 100;

    private readonly ConcurrentDictionary<long, RateLimitState> _states = new();

    /// <inheritdoc/>
    public bool IsBlocked(long installationId)
    {
        if (!_states.TryGetValue(installationId, out var state)) return false;
        return state.BlockedUntil.HasValue && clock.GetUtcNow() < state.BlockedUntil.Value;
    }

    /// <inheritdoc/>
    public void Update(long installationId, int? remaining, int? retryAfter, DateTimeOffset? resetAt)
    {
        _states.AddOrUpdate(
            installationId,
            _ => BuildState(remaining, retryAfter, resetAt),
            (_, _) => BuildState(remaining, retryAfter, resetAt));
    }

    /// <inheritdoc/>
    public void Clear(long installationId)
    {
        _states.TryRemove(installationId, out _);
    }

    private RateLimitState BuildState(int? remaining, int? retryAfter, DateTimeOffset? resetAt)
    {
        DateTimeOffset? blockedUntil = null;

        if (retryAfter.HasValue)
        {
            // Secondary rate-limit: block until Retry-After seconds from now
            blockedUntil = clock.GetUtcNow() + TimeSpan.FromSeconds(retryAfter.Value);
        }
        else if (remaining.HasValue && remaining.Value < LowWatermark && resetAt.HasValue)
        {
            // Primary rate-limit: block until reset time
            blockedUntil = resetAt.Value;
        }

        return new RateLimitState(blockedUntil);
    }

    private sealed record RateLimitState(DateTimeOffset? BlockedUntil);
}
