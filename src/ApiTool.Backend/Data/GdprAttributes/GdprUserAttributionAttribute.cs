namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>
/// Marks the property on an <c>InExport</c> entity that holds the user-attribution column.
/// Applied to exactly one property per entity in the <c>InExport</c> subset of the GDPR manifest.
/// The <see cref="GdprAttributeScanner"/> reads this attribute to populate
/// <see cref="Compliance.Gdpr.GdprManifestEntry.UserIdColumnName"/>, enabling the M18-004
/// export builder to filter per-table rows by the correct user-identifier column.
/// </summary>
[AttributeUsage(AttributeTargets.Property, AllowMultiple = false, Inherited = false)]
public sealed class GdprUserAttributionAttribute : Attribute;
