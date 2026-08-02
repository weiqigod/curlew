namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Implements the key rotation lifecycle for <see cref="ISigningKeyRotator"/>.
/// <para>
/// Normal: <c>next</c> → <c>current</c>; old <c>current</c> → <c>verifying</c>.
/// Emergency: <c>next</c> → <c>current</c>; old <c>current</c> → <c>revoked</c>.
/// When no <c>next</c> key exists and rotation is non-emergency, bootstraps a fresh current key.
/// When no <c>next</c> key exists and rotation IS emergency, revokes the current key immediately
/// and bootstraps a brand-new current key — the compromised key is never reused.
/// </para>
/// </summary>
public sealed class SigningKeyRotator(
    ISigningKeyStore store,
    IKeyProvider provider,
    TimeProvider clock) : ISigningKeyRotator
{
    /// <inheritdoc/>
    public async Task<string> RotateAsync(bool emergency, string? reason, CancellationToken ct = default)
    {
        var current = await store.LoadCurrentAsync(ct);
        var next = await store.LoadNextAsync(ct);

        if (next is null)
        {
            if (emergency && current is not null)
            {
                // Emergency with no staged key: revoke the compromised current key immediately,
                // then bootstrap a fresh current key. The revoked key's status is no longer
                // 'current', so the next GetActiveKidAsync call will bootstrap a new key pair.
                await store.UpdateStatusAsync(current.Kid, KeyStatus.Revoked, reason, clock, ct);
            }
            // Bootstrap a fresh current key (non-emergency) or after emergency revocation.
            return await provider.GetActiveKidAsync(ct);
        }

        var targetStatus = emergency ? KeyStatus.Revoked : KeyStatus.Verifying;

        // IMPORTANT: Demote old 'current' FIRST to free up the partial unique index slot,
        // then promote 'next' → 'current'. Reversing the order would trigger the constraint.
        if (current is not null)
        {
            await store.UpdateStatusAsync(current.Kid, targetStatus, reason, clock, ct);
        }

        // Promote 'next' → 'current'
        await store.UpdateStatusAsync(next.Kid, KeyStatus.Current, revokeReason: null, clock, ct);

        return next.Kid;
    }
}
