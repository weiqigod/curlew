namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Result from <see cref="VaultManifestValidator.Validate"/>.
/// Contains the JSON-path of every suspect field, or an empty list when the template is clean.
/// </summary>
/// <param name="OffendingPaths">
/// Dot-separated JSON paths of fields that match the literal-secret heuristic,
/// e.g. <c>team_secrets.vault_configs.staging.password</c>.
/// </param>
public sealed record VaultValidationResult(IReadOnlyList<string> OffendingPaths)
{
    /// <summary><see langword="true"/> when no suspicious fields were found.</summary>
    public bool Ok => OffendingPaths.Count == 0;
}
