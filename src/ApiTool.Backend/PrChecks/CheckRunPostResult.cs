namespace ApiTool.Backend.PrChecks;

/// <summary>Lifecycle status returned by <see cref="CheckRunPoster.PostAsync"/>.</summary>
public enum PrCheckPostStatus
{
    /// <summary>GitHub check run was created successfully.</summary>
    Posted,

    /// <summary>Post was deferred due to rate-limiting or transient error.</summary>
    Queued,

    /// <summary>Post failed permanently (bad config, repo not covered, etc.).</summary>
    Failed,

    /// <summary>An existing check run was found via idempotency GET; no duplicate post made.</summary>
    AlreadyPosted,
}

/// <summary>
/// Result of a single <see cref="CheckRunPoster.PostAsync"/> attempt.
/// </summary>
/// <param name="Status">Outcome of this attempt.</param>
/// <param name="CheckRunId">GitHub check_run id when <paramref name="Status"/> is Posted or AlreadyPosted. Null for GitLab provider rows.</param>
/// <param name="Error">Human-readable error message (tokens scrubbed).</param>
/// <param name="Code">Machine-readable error code for RFC-7807 mapping.</param>
public sealed record CheckRunPostResult(
    PrCheckPostStatus Status,
    long? CheckRunId,
    string? Error,
    PrCheckErrorCode? Code);
