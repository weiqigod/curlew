namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>GDPR disposition for a user-attributable column on an EF entity.</summary>
public enum GdprDisposition
{
    /// <summary>Row appears in /me/export; row is hard-deleted on user deletion.</summary>
    InExportInDeletionHard,

    /// <summary>Row appears in /me/export; the user-attribution column is anonymised on deletion.</summary>
    InExportInDeletionAnonymise,

    /// <summary>Row is excluded from export; the column is anonymised on deletion (row survives).</summary>
    ExcludedFromExportAnonymisedInDeletion,

    /// <summary>Row is excluded from both export and deletion path (no user-attribution column).</summary>
    ExcludedFromBoth,
}
