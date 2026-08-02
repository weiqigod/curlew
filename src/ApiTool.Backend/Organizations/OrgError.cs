namespace ApiTool.Backend.Organizations;

/// <summary>Well-known error codes returned by <see cref="OrganizationService"/>.</summary>
public enum OrgError
{
    /// <summary>No error — the operation succeeded.</summary>
    None,

    /// <summary>The supplied slug does not match the allowed format.</summary>
    InvalidSlug,

    /// <summary>Another organization already uses the supplied slug.</summary>
    SlugTaken,

    /// <summary>The requested organization was not found, or the user is not a member.</summary>
    NotFound,

    /// <summary>The supplied name is empty, whitespace, or exceeds the maximum length.</summary>
    InvalidName,
}
