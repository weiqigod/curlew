namespace ApiTool.Backend.Data.GdprAttributes;

/// <summary>
/// Marks a user-attribution property with its GDPR export and deletion disposition.
/// Apply to <c>UserId</c>, <c>ActorId</c>, <c>CreatedBy</c>, and similar columns.
/// Consumed by <see cref="ApiTool.Backend.Compliance.Gdpr.GdprAttributeScanner"/>.
/// </summary>
[AttributeUsage(AttributeTargets.Property, AllowMultiple = false, Inherited = false)]
public sealed class GdprIncludedAttribute(GdprDisposition disposition) : Attribute
{
    /// <summary>The GDPR disposition for the annotated property.</summary>
    public GdprDisposition Disposition { get; } = disposition;
}
