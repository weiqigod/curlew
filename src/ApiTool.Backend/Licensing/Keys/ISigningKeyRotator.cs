namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Orchestrates the key rotation lifecycle: promotes <c>next</c> to <c>current</c>,
/// demotes the old <c>current</c> to <c>verifying</c> (or <c>revoked</c> in emergency).
/// </summary>
public interface ISigningKeyRotator
{
    /// <summary>
    /// Rotates the active signing key.
    /// <para>
    /// Normal rotation: <c>next</c> → <c>current</c>; old <c>current</c> → <c>verifying</c>.
    /// Emergency rotation: <c>next</c> → <c>current</c>; old <c>current</c> → <c>revoked</c>
    /// with <paramref name="reason"/> recorded.
    /// </para>
    /// </summary>
    /// <returns>The kid of the newly active key.</returns>
    Task<string> RotateAsync(bool emergency, string? reason, CancellationToken ct = default);
}
