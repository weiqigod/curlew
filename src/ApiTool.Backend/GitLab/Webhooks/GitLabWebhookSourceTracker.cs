// Refs M16-015 task YAML behavior #6, plan Decision C.
using System.Collections.Concurrent;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Thread-safe in-process tracker for per-source-IP verification-failure quarantine.
/// After <see cref="QuarantineThreshold"/> consecutive failures from the same IP,
/// the source is quarantined and every subsequent request returns 429 until manually reset.
/// State is process-local; it is lost on restart (Decision C — acceptable for DoS protection).
/// Refs M16-015 task YAML behavior #6.
/// </summary>
/// <remarks>
/// TODO (follow-up): persist quarantine state to survive restarts if real-world incidents warrant it.
/// TODO (follow-up): add periodic eviction of idle non-quarantined entries to bound memory under attack.
/// </remarks>
public sealed class GitLabWebhookSourceTracker(TimeProvider clock)
{
    /// <summary>Number of consecutive verification failures that trigger quarantine.</summary>
    public const int QuarantineThreshold = 5;

    /// <summary>
    /// Idle window after which a warm (non-zero, non-quarantined) failure counter is reset.
    /// Prevents long-tail false positives from operators with intermittent misconfiguration.
    /// </summary>
    public static readonly TimeSpan IdleResetWindow = TimeSpan.FromMinutes(5);

    private readonly ConcurrentDictionary<string, SourceState> _states = new(StringComparer.Ordinal);

    /// <summary>
    /// Returns the current quarantine state for <paramref name="sourceIp"/>.
    /// </summary>
    /// <param name="sourceIp">The client IP address string (may be "unknown").</param>
    /// <returns>
    /// <c>Quarantined</c> = true when the IP has been quarantined; <c>QuarantinedAt</c> is
    /// the UTC moment of quarantine (null when not quarantined).
    /// </returns>
    public (bool Quarantined, DateTime? QuarantinedAt) GetState(string sourceIp)
    {
        if (_states.TryGetValue(sourceIp, out var state) && state.QuarantinedAt.HasValue)
            return (true, state.QuarantinedAt);
        return (false, null);
    }

    /// <summary>
    /// Records a single verification failure for <paramref name="sourceIp"/>.
    /// Returns the new failure counter value.
    /// When the counter reaches <see cref="QuarantineThreshold"/>, the IP is quarantined.
    /// Already-quarantined IPs do not increment further.
    /// An idle-reset resets a warm counter to 1 (the current failure) instead of accumulating.
    /// </summary>
    public int RecordFailure(string sourceIp)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        var newState = _states.AddOrUpdate(
            sourceIp,
            // Add: first failure for this IP
            _ => new SourceState(1, now, null),
            // Update: existing entry
            (_, existing) =>
            {
                // Already quarantined — don't change the counter
                if (existing.QuarantinedAt.HasValue)
                    return existing;

                // Idle-reset: last activity is older than the idle window → treat this as first failure
                var failureCount = (now - existing.LastFailureAt) > IdleResetWindow
                    ? 1
                    : existing.Failures + 1;

                if (failureCount >= QuarantineThreshold)
                    return new SourceState(failureCount, now, now);

                return new SourceState(failureCount, now, null);
            });

        return newState.Failures;
    }

    /// <summary>
    /// Records a successful verification for <paramref name="sourceIp"/>,
    /// resetting the failure counter to zero. Does NOT clear an existing quarantine —
    /// quarantine requires explicit <see cref="Reset"/> (future admin endpoint).
    /// </summary>
    public void RecordSuccess(string sourceIp)
    {
        _states.AddOrUpdate(
            sourceIp,
            _ => new SourceState(0, clock.GetUtcNow().UtcDateTime, null),
            (_, existing) => existing with { Failures = 0 });
    }

    /// <summary>
    /// Clears all state (failure counter and quarantine) for <paramref name="sourceIp"/>.
    /// Intended for a future admin-clear endpoint.
    /// </summary>
    public void Reset(string sourceIp) => _states.TryRemove(sourceIp, out _);

    private sealed record SourceState(int Failures, DateTime LastFailureAt, DateTime? QuarantinedAt);
}
