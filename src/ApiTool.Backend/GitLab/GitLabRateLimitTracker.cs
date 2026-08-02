// Refs M16-014 (per-installation GitLab rate-limit tracker, Guid-keyed).
using System.Collections.Concurrent;

namespace ApiTool.Backend.GitLab;

/// <summary>
/// Per-installation GitLab API rate-limit tracker.
/// Blocks new posts when <c>RateLimit-Remaining</c> drops to 0 or
/// when a <c>Retry-After</c> header is received.
/// Mirrors the shape of <see cref="ApiTool.Backend.GitHub.RateLimitTracker"/> but uses
/// <see cref="Guid"/> keys (gitlab_installations.Id) instead of <c>long</c>.
/// </summary>
public sealed class GitLabRateLimitTracker(TimeProvider clock) : IGitLabRateLimitTracker
{
    // Low-watermark: block when remaining <= 0 (GitLab uses 0, not 100).
    private const int LowWatermark = 0;

    private readonly ConcurrentDictionary<Guid, RateLimitState> _states = new();

    /// <inheritdoc/>
    public bool IsBlocked(Guid installationId)
    {
        if (!_states.TryGetValue(installationId, out var state)) return false;
        return state.BlockedUntil.HasValue && clock.GetUtcNow() < state.BlockedUntil.Value;
    }

    /// <inheritdoc/>
    public void Update(Guid installationId, int? remaining, int? retryAfter, DateTimeOffset? resetAt)
    {
        _states.AddOrUpdate(
            installationId,
            _ => BuildState(remaining, retryAfter, resetAt),
            (_, _) => BuildState(remaining, retryAfter, resetAt));
    }

    /// <inheritdoc/>
    public void Clear(Guid installationId)
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
        else if (remaining.HasValue && remaining.Value <= LowWatermark && resetAt.HasValue)
        {
            // Primary rate-limit: block until reset time
            blockedUntil = resetAt.Value;
        }

        return new RateLimitState(blockedUntil);
    }

    private sealed record RateLimitState(DateTimeOffset? BlockedUntil);
}
