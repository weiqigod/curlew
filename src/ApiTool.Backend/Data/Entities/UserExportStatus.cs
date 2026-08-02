namespace ApiTool.Backend.Data.Entities;

/// <summary>Tracks the lifecycle of a <see cref="UserExportRequest"/>.</summary>
public enum UserExportStatus
{
    /// <summary>The request has been received and is awaiting processing.</summary>
    Queued,

    /// <summary>The builder has claimed the request and is assembling the bundle.</summary>
    Building,

    /// <summary>The bundle is ready; <c>object_key</c> and <c>expires_at</c> are populated.</summary>
    Ready,

    /// <summary>Bundle assembly failed; <c>failure_reason</c> carries the error type.</summary>
    Failed,

    /// <summary>The signed URL has elapsed its 24h TTL; the caller must re-request.</summary>
    Expired,
}
