using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>A single column entry within a GDPR manifest entity record.</summary>
public sealed record GdprColumnEntry(
    string PropertyName,
    GdprDisposition Disposition,
    AnonymiseAs? AnonymiseAs);

/// <summary>
/// A manifest entry for one EF entity, carrying the table's overall disposition
/// and the list of annotated columns.
/// </summary>
public sealed record GdprManifestEntry(
    string TableName,
    Type EntityType,
    GdprDisposition Disposition,
    IReadOnlyList<GdprColumnEntry> Columns,
    string? UserIdColumnName = null);
