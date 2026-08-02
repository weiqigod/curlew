namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Service-layer error result for vault-config operations.
/// Distinct from <see cref="ApiTool.Backend.Internal.TierGates.VaultConfigError"/> which
/// represents tier-gate outcomes; this enum represents service-layer outcomes.
/// </summary>
public enum VaultConfigServiceError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>The organization or the vault-config row does not exist.</summary>
    NotFound,

    /// <summary>The requesting user lacks the required permission.</summary>
    PermissionDenied,

    /// <summary>
    /// The submitted template contains a field matching the literal-secret heuristic
    /// and the validator is in reject mode.
    /// </summary>
    SuspiciousValue,

    /// <summary>The submitted YAML could not be parsed.</summary>
    InvalidYaml,

    /// <summary>
    /// The persisted ciphertext could not be decrypted (wrong kid after key rotation,
    /// tampered ciphertext, or provider failure). Callers should surface a 500 to the client.
    /// </summary>
    DecryptionFailed,
}
