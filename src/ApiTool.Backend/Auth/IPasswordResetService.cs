namespace ApiTool.Backend.Auth;

/// <summary>
/// Password-reset service. Exposes idempotent <see cref="RequestAsync"/>
/// (always succeeds; rate-limit and unknown-email paths are silent) and
/// transactional <see cref="ConfirmAsync"/> that revokes refresh families.
/// </summary>
public interface IPasswordResetService
{
    /// <summary>
    /// Always returns success. Never tells the caller whether the email exists.
    /// Internally: looks up the user, checks per-email rate limit, mints+stores a
    /// hashed token, and enqueues the password_reset email.
    /// </summary>
    Task RequestAsync(string email, string? requesterIp, string? requesterUa, CancellationToken ct);

    /// <summary>
    /// Verifies the token, scores the new password, updates the user's password_hash,
    /// consumes the token, and revokes all refresh-token families for the user.
    /// Returns <see cref="PasswordResetError.None"/> on success; the score is set to the
    /// out parameter when the result is <see cref="PasswordResetError.PasswordTooWeak"/>.
    /// </summary>
    Task<(PasswordResetError Error, int? Score)> ConfirmAsync(
        string token, string newPassword, CancellationToken ct);
}
