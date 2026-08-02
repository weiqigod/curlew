using System.Reflection;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// CI guard: asserts that every entity in the <c>ApiTool.Backend.Data.Entities</c>
/// namespace that carries a user-attribution column is tagged with at least one
/// GDPR attribute. Fails loudly, naming the offending entity, when a new
/// user-attributable entity is added without GDPR attributes — closing the
/// future-drift gap called out in M18-003 (v4-4).
/// </summary>
public class GdprInventoryCoverageTests
{
    // The set of property names considered "user-attribution columns" for GDPR coverage.
    // Excluded: RequesterIp — forensic only, not a user-attribution column per Architecture Decision 5.
    private static readonly HashSet<string> UserAttributionPropertyNames = new(StringComparer.Ordinal)
    {
        "UserId", "ActorId", "CreatedBy", "UpdatedBy",
        "InvitedBy", "UploadedBy", "OwnerId",
    };

    public static IEnumerable<object[]> EntityTypes() =>
        typeof(AppDbContext).Assembly.GetTypes()
            .Where(t => t.IsClass && !t.IsAbstract && t.Namespace == "ApiTool.Backend.Data.Entities")
            .Select(t => new object[] { t });

    [Theory]
    [MemberData(nameof(EntityTypes))]
    public void Every_user_attributable_entity_is_tagged(Type entityType)
    {
        bool hasUserAttributionColumn = entityType
            .GetProperties(BindingFlags.Public | BindingFlags.Instance)
            .Any(p => UserAttributionPropertyNames.Contains(p.Name));

        if (!hasUserAttributionColumn)
            return; // Not user-attributable; no GDPR tag required.

        var classAttr = entityType.GetCustomAttribute<GdprTableAttribute>();

        // Explicit opt-outs: only NotUserAttributable or ExcludedFromBoth skip the coverage
        // check. Switching on Kind explicitly closes the gap that would reappear if
        // GdprTableKind ever grew a non-exempting variant.
        if (classAttr is { Kind: GdprTableKind.NotUserAttributable or GdprTableKind.ExcludedFromBoth })
            return;

        bool tagged = entityType
            .GetProperties(BindingFlags.Public | BindingFlags.Instance)
            .Any(p =>
                p.GetCustomAttribute<GdprIncludedAttribute>() is not null
                || p.GetCustomAttribute<GdprAnonymiseAttribute>() is not null);

        tagged.Should().BeTrue(
            $"entity '{entityType.Name}' carries a user-attribution column " +
            $"({string.Join(", ", entityType.GetProperties(BindingFlags.Public | BindingFlags.Instance).Where(p => UserAttributionPropertyNames.Contains(p.Name)).Select(p => p.Name))}) " +
            $"but has no [GdprIncluded] / [GdprAnonymise] / [GdprTable(...)] attribute — " +
            $"add the appropriate GDPR attribute (see docs/security/data-inventory.md).");
    }
}
