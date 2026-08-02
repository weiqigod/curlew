namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>Classifies a table's GDPR standing at the class level.</summary>
public enum GdprTableKind
{
    /// <summary>
    /// Listed in the v4-4 data inventory but carries no user-attribution column.
    /// The scanner synthesises a manifest entry with <see cref="GdprDisposition.ExcludedFromBoth"/>.
    /// </summary>
    ExcludedFromBoth,

    /// <summary>
    /// Not user-attributable for GDPR purposes (e.g. <c>Organization</c>, <c>Result</c>).
    /// The coverage test skips this entity; it does not appear in the manifest.
    /// </summary>
    NotUserAttributable,
}

/// <summary>
/// Class-level GDPR marker for entities that either have no user-attribution column
/// (<see cref="GdprTableKind.ExcludedFromBoth"/>) or are explicitly out of scope
/// (<see cref="GdprTableKind.NotUserAttributable"/>).
/// </summary>
[AttributeUsage(AttributeTargets.Class, AllowMultiple = false, Inherited = false)]
public sealed class GdprTableAttribute(GdprTableKind kind) : Attribute
{
    /// <summary>The GDPR classification for the annotated entity class.</summary>
    public GdprTableKind Kind { get; } = kind;
}
