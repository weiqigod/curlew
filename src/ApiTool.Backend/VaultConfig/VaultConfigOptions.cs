namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Options for the vault-config feature. Bound from the configuration section
/// <c>ApiTool:VaultConfig</c> (env prefix <c>APITOOL__VAULTCONFIG__</c>).
/// </summary>
public sealed class VaultConfigOptions
{
    /// <summary>Configuration section key.</summary>
    public const string Section = "ApiTool:VaultConfig";

    /// <summary>
    /// Controls how the manifest validator handles detected literal secrets.
    /// <list type="bullet">
    ///   <item><c>reject</c> — returns 422 (production-safe default).</item>
    ///   <item><c>warn</c> — saves the template and logs a warning (development).</item>
    /// </list>
    /// </summary>
    public string ValidatorMode { get; set; } = "reject";

    /// <summary><see langword="true"/> when the validator is in reject mode.</summary>
    public bool IsRejectMode => string.Equals(ValidatorMode, "reject", StringComparison.OrdinalIgnoreCase);
}
