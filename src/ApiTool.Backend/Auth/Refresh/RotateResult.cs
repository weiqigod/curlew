using ApiTool.Backend.Data.Entities;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>Discriminated-union result for <see cref="RefreshTokenService.RotateAsync"/>.</summary>
public enum RotateOutcome
{
    /// <summary>Token accepted and rotated; new token issued.</summary>
    Success,

    /// <summary>Token was already rotated — reuse attack detected; family revoked.</summary>
    Reused,

    /// <summary>Token is past its absolute expiry deadline.</summary>
    Expired,

    /// <summary>Presented device_id does not match the token's bound device_id.</summary>
    DeviceMismatch,

    /// <summary>Token hash not found in the database.</summary>
    NotFound,

    /// <summary>Token has been explicitly revoked (e.g., by a prior reuse-detection sweep).</summary>
    Revoked,
}

/// <summary>Result returned by <see cref="RefreshTokenService.RotateAsync"/>.</summary>
public sealed record RotateResult(
    RotateOutcome Outcome,
    RefreshToken? NewRow,          // populated when Outcome=Success
    string? NewPlaintextToken,     // populated when Outcome=Success
    Guid? UserId,                  // populated for any outcome that loaded a row
    Guid? FamilyId);               // populated for any outcome that loaded a row
