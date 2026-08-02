using System.Reflection;
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>Sanity tests asserting AttributeUsage shape and round-trip of ctor args.</summary>
public class GdprAttributesTests
{
    [Theory]
    [InlineData(typeof(GdprIncludedAttribute), AttributeTargets.Property)]
    [InlineData(typeof(GdprAnonymiseAttribute), AttributeTargets.Property)]
    [InlineData(typeof(GdprTableAttribute), AttributeTargets.Class)]
    public void Attribute_targets_are_correct(Type attrType, AttributeTargets expected)
    {
        var usage = attrType.GetCustomAttribute<AttributeUsageAttribute>(inherit: true)!;
        usage.ValidOn.Should().Be(expected);
        usage.AllowMultiple.Should().BeFalse();
    }

    [Fact]
    public void GdprIncludedAttribute_round_trips_disposition() =>
        new GdprIncludedAttribute(GdprDisposition.InExportInDeletionAnonymise)
            .Disposition.Should().Be(GdprDisposition.InExportInDeletionAnonymise);

    [Fact]
    public void GdprAnonymiseAttribute_round_trips_kind() =>
        new GdprAnonymiseAttribute(AnonymiseAs.SetNull)
            .Kind.Should().Be(AnonymiseAs.SetNull);

    [Fact]
    public void GdprDisposition_has_exactly_four_values() =>
        Enum.GetValues<GdprDisposition>().Should().HaveCount(4);

    [Fact]
    public void AnonymiseAs_has_exactly_two_values() =>
        Enum.GetValues<AnonymiseAs>().Should().HaveCount(2);

    [Fact]
    public void GdprTableAttribute_round_trips_kind() =>
        new GdprTableAttribute(GdprTableKind.NotUserAttributable)
            .Kind.Should().Be(GdprTableKind.NotUserAttributable);

    [Fact]
    public void GdprTableKind_has_exactly_two_values() =>
        Enum.GetValues<GdprTableKind>().Should().HaveCount(2);
}
