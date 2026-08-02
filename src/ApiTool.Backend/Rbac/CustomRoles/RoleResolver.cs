using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>
/// Computes the effective permission set for a given organization member.
/// Returns an empty set if the user is not a member of the org.
/// </summary>
/// <remarks>
/// Algorithm:
/// <list type="number">
///   <item>Look up the <see cref="OrganizationMember"/> row; if null → empty set.</item>
///   <item>If <c>RoleId</c> is non-null and the referenced <see cref="CustomRole"/> exists in the same org
///         → deserialize <c>PermissionsJson</c> → base set.</item>
///   <item>Otherwise → pick built-in set from <see cref="Permissions"/> by the member's
///         <see cref="OrgRole"/>.</item>
///   <item>Union with direct overrides parsed from <see cref="OrganizationMember.PermissionsJson"/>.</item>
///   <item>Ignore unknown permission keys silently.</item>
/// </list>
/// </remarks>
public sealed class RoleResolver(AppDbContext db)
{
    /// <summary>
    /// Returns the effective set of permission keys for <paramref name="userId"/> in <paramref name="orgId"/>.
    /// </summary>
    public async Task<IReadOnlySet<string>> GetEffectivePermissionsAsync(
        Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

        if (member is null)
            return new HashSet<string>(StringComparer.Ordinal);

        // Determine base set.
        IEnumerable<string> basePerms;

        if (member.RoleId is { } roleId)
        {
            var customRole = await db.OrganizationCustomRoles
                .FirstOrDefaultAsync(r => r.Id == roleId && r.OrgId == orgId, ct);

            if (customRole is not null)
                basePerms = DeserializeKeys(customRole.PermissionsJson);
            else
                // RoleId points to a deleted or cross-org role; fall back to built-in.
                basePerms = GetBuiltInSet(member.Role);
        }
        else
        {
            basePerms = GetBuiltInSet(member.Role);
        }

        // Union with direct overrides, filtering unknown keys.
        var result = new HashSet<string>(StringComparer.Ordinal);
        foreach (var k in basePerms)
        {
            if (Permissions.All.Contains(k))
                result.Add(k);
        }

        foreach (var k in DeserializeKeys(member.PermissionsJson))
        {
            if (Permissions.All.Contains(k))
                result.Add(k);
        }

        return result;
    }

    /// <summary>
    /// Returns <see langword="true"/> when <paramref name="userId"/> has
    /// <paramref name="permissionKey"/> in their effective permission set.
    /// </summary>
    public async Task<bool> HasPermissionAsync(
        Guid userId, Guid orgId, string permissionKey, CancellationToken ct)
    {
        var perms = await GetEffectivePermissionsAsync(userId, orgId, ct);
        return perms.Contains(permissionKey);
    }

    private static IEnumerable<string> GetBuiltInSet(OrgRole role) => role switch
    {
        OrgRole.Owner => Permissions.BuiltInOwner,
        OrgRole.Admin => Permissions.BuiltInAdmin,
        OrgRole.Member => Permissions.BuiltInMember,
        _ => Permissions.BuiltInMember,
    };

    private static IEnumerable<string> DeserializeKeys(string json)
    {
        try
        {
            return JsonSerializer.Deserialize<List<string>>(json) ?? [];
        }
        catch (JsonException)
        {
            return [];
        }
    }
}
