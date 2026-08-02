using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// The result of scanning an EF entity assembly for GDPR attributes.
/// Downstream consumers — M18-004 export builder and M18-006 anonymiser — read
/// this manifest instead of maintaining their own hardcoded entity lists.
/// </summary>
public sealed record GdprBundleManifest(IReadOnlyList<GdprManifestEntry> Entries)
{
    /// <summary>Entries whose rows appear in the per-user export bundle.</summary>
    public IEnumerable<GdprManifestEntry> InExport =>
        Entries.Where(e =>
            e.Disposition is GdprDisposition.InExportInDeletionHard
                          or GdprDisposition.InExportInDeletionAnonymise);

    /// <summary>Entries whose rows are hard-deleted on user deletion.</summary>
    public IEnumerable<GdprManifestEntry> InDeletionHard =>
        Entries.Where(e => e.Disposition == GdprDisposition.InExportInDeletionHard);

    /// <summary>
    /// Entries whose <em>table-level</em> disposition is anonymise-on-deletion. This is the
    /// per-table view: an entry is included if its table disposition is
    /// <see cref="GdprDisposition.InExportInDeletionAnonymise"/> or
    /// <see cref="GdprDisposition.ExcludedFromExportAnonymisedInDeletion"/>.
    /// Note that the table disposition is the minimum (strongest-inclusion) disposition
    /// across the entity's columns, so this list may omit entries that have <em>some</em>
    /// anonymisable columns but a stronger table-level disposition (e.g. <c>OrganizationMember</c>
    /// is <c>InExportInDeletionHard</c> at the table level, but its <c>InvitedBy</c> column is
    /// anonymisable). Consumers that need the full column-level view should use
    /// <see cref="ColumnsToAnonymise"/>.
    /// </summary>
    public IEnumerable<GdprManifestEntry> TablesAnonymisedOnDeletion =>
        Entries.Where(e =>
            e.Disposition is GdprDisposition.InExportInDeletionAnonymise
                          or GdprDisposition.ExcludedFromExportAnonymisedInDeletion);

    /// <summary>
    /// Every (entity, column) pair carrying a <see cref="GdprColumnEntry.AnonymiseAs"/>
    /// value, flattened across all manifest entries. This is the column-level view
    /// consumed by M18-006's anonymiser — every yielded pair must have its column NULL'd
    /// or token-substituted regardless of the parent table's disposition.
    /// </summary>
    public IEnumerable<(GdprManifestEntry Entry, GdprColumnEntry Column)> ColumnsToAnonymise =>
        Entries.SelectMany(e => e.Columns
            .Where(c => c.AnonymiseAs is not null)
            .Select(c => (e, c)));

    /// <summary>Entries excluded from both the export and the deletion anonymisation path.</summary>
    public IEnumerable<GdprManifestEntry> ExcludedFromBoth =>
        Entries.Where(e => e.Disposition == GdprDisposition.ExcludedFromBoth);

    /// <summary>Returns the manifest entry for the given entity type, or <see langword="null"/> if absent.</summary>
    public GdprManifestEntry? FindByEntity(Type t) =>
        Entries.FirstOrDefault(e => e.EntityType == t);
}
