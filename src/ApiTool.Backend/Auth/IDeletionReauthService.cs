namespace ApiTool.Backend.Auth;

/// <summary>
/// Issues and consumes single-use 5-minute re-authentication tokens (prefix <c>drto_</c>)
/// that gate <c>POST /api/v1/users/me/deletion-requests</c> and other destructive operations.
/// Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public interface IDeletionReauthService
{
    /// <summary>
    /// Verifies the user's password and, if correct, mints a new <c>drto_</c> token
    /// persisted as a SHA-256 hash in <c>deletion_reauth_tokens</c>.
    /// </summary>
    /// <param name="userId">The authenticated user's id.</param>
    /// <param name="password">The plaintext password to verify against the stored hash.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// <c>(None, rawToken, expiresAt)</c> on success;
    /// <c>(WrongPassword, null, null)</c> when the password does not match.
    /// </returns>
    Task<(ReauthError Error, string? Token, DateTime? ExpiresAt)> IssueAsync(
        Guid userId, string password, CancellationToken ct);

    /// <summary>
    /// Validates and atomically consumes a <c>drto_</c> token presented in
    /// <c>X-Reauth-Token</c>. Sets <c>consumed_at</c> so a second call returns
    /// <see cref="ReauthError.TokenConsumed"/> (single-use semantics).
    /// </summary>
    /// <param name="userId">The JWT-authenticated user's id — must match the token's owner.</param>
    /// <param name="rawToken">The raw prefixed token from the request header.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// <see cref="ReauthError.None"/> on success;
    /// <see cref="ReauthError.TokenInvalid"/> when token is not found or belongs to a different user;
    /// <see cref="ReauthError.TokenExpired"/> when the 5-minute TTL has elapsed;
    /// <see cref="ReauthError.TokenConsumed"/> when the token was already used.
    /// </returns>
    Task<ReauthError> ConsumeAsync(Guid userId, string rawToken, CancellationToken ct);
}
