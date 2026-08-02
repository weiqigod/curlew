namespace ApiTool.Backend.PrChecks;

/// <summary>Error codes for PR-check operations.</summary>
public enum PrCheckError
{
    None,
    PermissionDenied,
    InvalidState,
    ResultNotFound,
}
