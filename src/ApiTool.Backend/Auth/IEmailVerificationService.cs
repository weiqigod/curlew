namespace ApiTool.Backend.Auth;

/// <summary>
/// Email-verification service. Exposes a resend path that rotates existing tokens
/// and a confirm path that marks the user's email as verified.
/// </summary>
public interface IEmailVerificationService
{
    /// <summary>
    /// Revokes any non-consumed verification token rows for the user (per spec :8509),
    /// inserts a fresh evtk_ row with 24h expiry, and enqueues the email_verification email.
    /// Silent on unknown email (enumeration defense) and on per-email rate-limit hit.
    /// </summary>
    Task ResendAsync(string email, CancellationToken ct);

    /// <summary>
    /// Verifies the token, sets users.email_verified = true, sets consumed_at = NOW().
    /// </summary>
    Task<EmailVerificationError> ConfirmAsync(string token, CancellationToken ct);
}
