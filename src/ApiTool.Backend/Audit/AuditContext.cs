namespace ApiTool.Backend.Audit;

/// <summary>Per-request holder of audit-relevant request metadata, populated by
/// <see cref="AuditCaptureMiddleware"/> and consumed by <see cref="IAuditWriter"/>.</summary>
public sealed class AuditContext
{
    /// <summary>The IP address of the requester, resolved from <c>X-Forwarded-For</c> or the
    /// socket peer address. <see langword="null"/> when no IP is available.</summary>
    public string? IpAddress { get; set; }

    /// <summary>The <c>User-Agent</c> header value, truncated to 500 characters.
    /// Empty string when the header is absent.</summary>
    public string? UserAgent { get; set; }
}
