// Spec refs: docs/SPECIFICATION.md:8924-8933 (M14 inventory) and :8584-8589 (M16 additions).
namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Canonical list of transactional email template slugs shipped by the backend.
/// Originally six M14 slugs; M16 adds <c>password_reset</c> and <c>trial_expiring</c>
/// (deferred from M14 per spec :9565 because no M14 slice exercised them).
/// </summary>
public static class EmailTemplateInventory
{
    /// <summary>
    /// All shipped slugs. Adding to this list requires also adding the matching
    /// <c>templates/email/&lt;slug&gt;.{mjml,json}</c> pair and updating
    /// docs/SPECIFICATION.md.
    /// </summary>
    public static IReadOnlyList<string> Slugs { get; } = new[]
    {
        "email_verification",
        "auth_device_code",
        "billing_receipt",
        "billing_payment_failed",
        "billing_subscription_canceled",
        "account_security_alert",
        // M16 additions (spec :8584-8589):
        "password_reset",
        "trial_expiring",
        // M18-005 additions (v4-5 — GDPR account deletion state machine):
        "account_deletion_initiated",
        "account_deletion_completed",
    };
}
