namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Anonymises a user's personal data to fulfil a GDPR deletion request.
/// The stub implementation (M18-005) sets <c>users.anonymised_at</c> and clears
/// <c>users.pending_deletion_at</c>; the full concrete implementation (M18-006) will
/// also scrub every PII column listed in <see cref="GdprBundleManifest"/>.
/// Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public interface IUserAnonymiser
{
    /// <summary>
    /// Anonymises all personal data for <paramref name="userId"/>.
    /// Callers must snapshot the user's email BEFORE invoking this method if they
    /// need to send a post-deletion email (M18-006 prerequisite).
    /// </summary>
    Task AnonymiseAsync(Guid userId, CancellationToken ct);
}
