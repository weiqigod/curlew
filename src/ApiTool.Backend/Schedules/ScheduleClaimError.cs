namespace ApiTool.Backend.Schedules;

/// <summary>Business-outcome error enum for schedule executor operations.</summary>
/// <remarks>
/// Distinct from <see cref="ApiTool.Backend.Internal.TierGates.ScheduleExecutorError"/>, which is
/// the tier-gate outcome. This enum covers claim / heartbeat / result business cases.
/// </remarks>
public enum ScheduleClaimError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>No queued runs available for the org — respond 204 No Content.</summary>
    NoRunsAvailable,

    /// <summary>The run id was not found or does not belong to the caller's org — respond 404.</summary>
    NotFound,

    /// <summary>
    /// The claim token has been cleared (row reaped back to Queued) — respond 409
    /// with <c>type: .../claim-reaped</c>.
    /// </summary>
    ClaimReaped,

    /// <summary>
    /// The presented claim token does not match the stored token — respond 409.
    /// </summary>
    StaleClaim,

    /// <summary>
    /// The run is already in a terminal state (Completed or Failed) — respond 409.
    /// </summary>
    AlreadyCompleted,

    /// <summary>Request payload is invalid (missing claim_token, malformed body) — respond 400.</summary>
    InvalidRequest,

    /// <summary>
    /// Reserved for future use — result persistence or other unexpected server failures.
    /// Currently no code path returns this; the endpoint wildcard arm (<c>_ =&gt; 500</c>) handles any future additions.
    /// </summary>
    InternalError,

    /// <summary>
    /// The schedule's env_vars ciphertext could not be decrypted (wrong kid after key rotation,
    /// tampered ciphertext, or provider failure). The run is left in Running state so the worker
    /// can retry; the caller receives a 500 to avoid silently delivering an incorrect env-var set.
    /// </summary>
    DecryptionFailed,
}
