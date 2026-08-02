namespace ApiTool.Backend.Audit;

/// <summary>Configuration for <see cref="AuditLogCleanupHost"/>.</summary>
public sealed class AuditLogCleanupOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:AuditLog:Cleanup";

    /// <summary>How often to run the cleanup tick. Defaults to 24 hours.</summary>
    public TimeSpan TickInterval { get; set; } = TimeSpan.FromHours(24);
}
