namespace ApiTool.Backend.GitHub.Installations;

/// <summary>Sentinel error type for github_installations claim/upsert operations.</summary>
public enum InstallClaimError
{
    /// <summary>Operation succeeded.</summary>
    None,

    /// <summary>
    /// The org already has a different active installation (cross-tenant UNIQUE index violation,
    /// spec :8427). Callers should return HTTP 409.
    /// </summary>
    AlreadyLinked,

    /// <summary>The installation_id was not found in the database.</summary>
    NotFound,

    /// <summary>The caller does not have permission to perform the operation.</summary>
    NotPermitted,
}
