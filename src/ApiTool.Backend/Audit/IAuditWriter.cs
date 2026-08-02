namespace ApiTool.Backend.Audit;

/// <summary>
/// Appends an audit log row to the pending DbContext transaction.
/// Callers must still call <c>SaveChangesAsync</c> to flush the row.
/// </summary>
public interface IAuditWriter
{
    /// <summary>Enqueues a single audit row. Does not flush — caller is responsible for
    /// calling <c>SaveChangesAsync</c>.</summary>
    void Append(AuditEvent evt);
}
