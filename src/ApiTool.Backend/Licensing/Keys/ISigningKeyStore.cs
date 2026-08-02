using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Database I/O interface for <see cref="SigningKey"/> rows.
/// Kept separate from <see cref="IKeyProvider"/> so DB access can be mocked without
/// re-implementing crypto in tests.
/// </summary>
public interface ISigningKeyStore
{
    /// <summary>Inserts a new signing key row.</summary>
    Task InsertAsync(SigningKey row, CancellationToken ct = default);

    /// <summary>Returns the key with the given kid, or null if not found.</summary>
    Task<SigningKey?> LoadByKidAsync(string kid, CancellationToken ct = default);

    /// <summary>Returns the key with <c>status='current'</c>, or null if none exists.</summary>
    Task<SigningKey?> LoadCurrentAsync(CancellationToken ct = default);

    /// <summary>Returns the key with <c>status='next'</c>, or null if none exists.</summary>
    Task<SigningKey?> LoadNextAsync(CancellationToken ct = default);

    /// <summary>Returns all keys with <c>status='current'</c> or <c>status='verifying'</c>.</summary>
    Task<IReadOnlyList<SigningKey>> LoadCurrentAndVerifyingAsync(CancellationToken ct = default);

    /// <summary>
    /// Updates the status of the key identified by <paramref name="kid"/>.
    /// Sets <paramref name="revokeReason"/> and <c>RevokedAt</c> when <paramref name="newStatus"/> is <c>revoked</c>.
    /// Returns true when a row was updated, false when the kid was not found.
    /// </summary>
    Task<bool> UpdateStatusAsync(string kid, string newStatus, string? revokeReason, TimeProvider clock, CancellationToken ct = default);
}
