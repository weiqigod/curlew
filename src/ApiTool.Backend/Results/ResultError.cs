namespace ApiTool.Backend.Results;

/// <summary>Well-known error codes returned by <see cref="ResultsService"/>.</summary>
public enum ResultError
{
    /// <summary>No error; operation succeeded.</summary>
    None,

    /// <summary>Caller is not an org member.</summary>
    PermissionDenied,

    /// <summary>Payload fails structural validation.</summary>
    InvalidSchema,

    /// <summary>Result does not exist or belongs to another org.</summary>
    NotFound,

    /// <summary>Body exceeded 5 MB (emitted by middleware, not service).</summary>
    PayloadTooLarge,
}
