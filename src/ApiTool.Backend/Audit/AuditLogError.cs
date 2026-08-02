namespace ApiTool.Backend.Audit;

/// <summary>Well-known error cases for the audit log query flow.</summary>
public enum AuditLogError
{
    /// <summary>No error.</summary>
    None,

    /// <summary>The caller does not have permission to read the audit log.</summary>
    PermissionDenied,

    /// <summary>A filter parameter was invalid (e.g. from &gt; to).</summary>
    InvalidFilter,
}
