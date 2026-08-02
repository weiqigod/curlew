namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>
/// Declares how a column should be anonymised during user deletion.
/// Optional companion to <see cref="GdprIncludedAttribute"/>; absence implies hard-delete.
/// Apply to <c>Guid?</c> columns (use <see cref="AnonymiseAs.SetNull"/>) or
/// <c>string?</c> columns (use <see cref="AnonymiseAs.DeletedUserToken"/>).
/// Consumed by M18-006's <c>IUserAnonymiser</c>.
/// </summary>
[AttributeUsage(AttributeTargets.Property, AllowMultiple = false, Inherited = false)]
public sealed class GdprAnonymiseAttribute(AnonymiseAs kind) : Attribute
{
    /// <summary>How the annotated column is anonymised.</summary>
    public AnonymiseAs Kind { get; } = kind;
}
