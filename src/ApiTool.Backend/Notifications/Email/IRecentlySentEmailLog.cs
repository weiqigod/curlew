namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Test-only recorder for successfully-sent <see cref="EmailMessage"/> entries.
/// Registered ONLY in Development and Testing environments — production
/// must not depend on this surface. Bounded buffer; oldest entries are
/// evicted at capacity.
/// </summary>
public interface IRecentlySentEmailLog
{
    /// <summary>Records a message that was successfully sent.</summary>
    void Record(EmailMessage message, DateTimeOffset sentAt);

    /// <summary>Returns the most recent entries, oldest first, up to <paramref name="limit"/>.</summary>
    IReadOnlyList<RecentlySentEmail> Recent(int limit);
}

/// <summary>A single audit row representing a successfully-sent email.</summary>
/// <param name="To">Recipient email address.</param>
/// <param name="TemplateSlug">SendGrid template slug (e.g. <c>billing_receipt</c>).</param>
/// <param name="SentAt">Server-side timestamp when the send completed.</param>
/// <param name="Variables">Template variables that were substituted.</param>
public sealed record RecentlySentEmail(
    string To,
    string TemplateSlug,
    DateTimeOffset SentAt,
    IReadOnlyDictionary<string, object> Variables);
