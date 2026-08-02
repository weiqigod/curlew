using System.Text.Json;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Rbac.CustomRoles;

/// <summary>Business logic for managing custom organization roles.</summary>
public sealed class CustomRolesService(AppDbContext db, TimeProvider clock, IAuditWriter audit)
{
    private static readonly HashSet<string> ReservedNames =
        new(["owner", "admin", "member"], StringComparer.OrdinalIgnoreCase);

    // Static built-in DTOs prepended to every list response.
    private static readonly IReadOnlyList<CustomRoleDto> BuiltIns =
    [
        new CustomRoleDto("owner", "owner", Permissions.BuiltInOwner.ToList(), IsBuiltin: true, CreatedAt: null),
        new CustomRoleDto("admin", "admin", Permissions.BuiltInAdmin.ToList(), IsBuiltin: true, CreatedAt: null),
        new CustomRoleDto("member", "member", Permissions.BuiltInMember.ToList(), IsBuiltin: true, CreatedAt: null),
    ];

    /// <summary>Lists all roles (three built-ins + custom) for the given organization.</summary>
    public async Task<(IReadOnlyList<CustomRoleDto> roles, CustomRoleError err)>
        ListAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return ([], CustomRoleError.OrganizationNotFound);

        var customs = await db.OrganizationCustomRoles
            .Where(r => r.OrgId == orgId)
            .OrderByDescending(r => r.CreatedAt)
            .ToListAsync(ct);

        var dtos = new List<CustomRoleDto>(BuiltIns.Count + customs.Count);
        dtos.AddRange(BuiltIns);
        foreach (var r in customs)
            dtos.Add(ToDto(r));

        return (dtos, CustomRoleError.None);
    }

    /// <summary>Creates a new custom role for the organization. Owner only.</summary>
    public async Task<(CustomRoleDto? dto, CustomRoleError err, string? message)>
        CreateAsync(Guid userId, Guid orgId, CreateRoleRequest req, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return (null, CustomRoleError.OrganizationNotFound, null);
        if (member.Role != OrgRole.Owner)
            return (null, CustomRoleError.PermissionDenied, null);

        var trimmed = req.Name.Trim();
        if (trimmed.Length == 0 || trimmed.Length > 100 || ReservedNames.Contains(trimmed))
            return (null, CustomRoleError.InvalidName, $"Invalid role name: '{req.Name}'.");

        // Validate all permission keys.
        var badKey = req.Permissions.FirstOrDefault(k => !Permissions.All.Contains(k));
        if (badKey is not null)
            return (null, CustomRoleError.InvalidPermission, badKey);

        // Check for duplicate name within the org.
        var exists = await db.OrganizationCustomRoles
            .AnyAsync(r => r.OrgId == orgId && r.Name == trimmed, ct);
        if (exists)
            return (null, CustomRoleError.RoleNameTaken, null);

        var now = clock.GetUtcNow().UtcDateTime;
        var roleId = Guid.NewGuid();
        var role = new CustomRole
        {
            Id = roleId,
            OrgId = orgId,
            Name = trimmed,
            PermissionsJson = JsonSerializer.Serialize(req.Permissions.ToList()),
            CreatedAt = now,
            CreatedBy = userId,
        };
        db.OrganizationCustomRoles.Add(role);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "role.created",
            TargetType: "custom_role",
            TargetId: roleId,
            Payload: new { name = trimmed, permissions = req.Permissions }));

        await db.SaveChangesAsync(ct);
        return (ToDto(role), CustomRoleError.None, null);
    }

    /// <summary>Fully replaces a custom role's name and permission set. Owner only.</summary>
    public async Task<(CustomRoleDto? dto, CustomRoleError err, string? message)>
        UpdateAsync(Guid userId, Guid orgId, Guid roleId, UpdateRoleRequest req, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return (null, CustomRoleError.OrganizationNotFound, null);
        if (member.Role != OrgRole.Owner)
            return (null, CustomRoleError.PermissionDenied, null);

        var role = await db.OrganizationCustomRoles
            .FirstOrDefaultAsync(r => r.Id == roleId && r.OrgId == orgId, ct);
        if (role is null)
            return (null, CustomRoleError.RoleNotFound, null);

        var trimmed = req.Name.Trim();
        if (trimmed.Length == 0 || trimmed.Length > 100 || ReservedNames.Contains(trimmed))
            return (null, CustomRoleError.InvalidName, $"Invalid role name: '{req.Name}'.");

        var badKey = req.Permissions.FirstOrDefault(k => !Permissions.All.Contains(k));
        if (badKey is not null)
            return (null, CustomRoleError.InvalidPermission, badKey);

        var nameTaken = await db.OrganizationCustomRoles
            .AnyAsync(r => r.OrgId == orgId && r.Id != roleId && r.Name == trimmed, ct);
        if (nameTaken)
            return (null, CustomRoleError.RoleNameTaken, null);

        var previousName = role.Name;
        var previousPermissions = role.PermissionsJson;
        role.Name = trimmed;
        role.PermissionsJson = JsonSerializer.Serialize(req.Permissions.ToList());

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "role.updated",
            TargetType: "custom_role",
            TargetId: roleId,
            Payload: new
            {
                previous_name = previousName,
                previous_permissions = JsonSerializer.Deserialize<List<string>>(previousPermissions) ?? [],
                name = trimmed,
                permissions = req.Permissions,
            }));

        await db.SaveChangesAsync(ct);
        return (ToDto(role), CustomRoleError.None, null);
    }

    /// <summary>
    /// Deletes a custom role. Returns <see cref="CustomRoleError.RoleInUse"/> with the member count
    /// when any members still reference the role. Owner only.
    /// </summary>
    public async Task<(CustomRoleError err, int memberCount)>
        DeleteAsync(Guid userId, Guid orgId, Guid roleId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (member is null)
            return (CustomRoleError.OrganizationNotFound, 0);
        if (member.Role != OrgRole.Owner)
            return (CustomRoleError.PermissionDenied, 0);

        var role = await db.OrganizationCustomRoles
            .FirstOrDefaultAsync(r => r.Id == roleId && r.OrgId == orgId, ct);
        if (role is null)
            return (CustomRoleError.RoleNotFound, 0);

        var count = await db.OrganizationMembers
            .CountAsync(m => m.OrgId == orgId && m.RoleId == roleId, ct);
        if (count > 0)
            return (CustomRoleError.RoleInUse, count);

        db.OrganizationCustomRoles.Remove(role);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "role.deleted",
            TargetType: "custom_role",
            TargetId: roleId,
            Payload: new { name = role.Name }));

        await db.SaveChangesAsync(ct);
        return (CustomRoleError.None, 0);
    }

    private static CustomRoleDto ToDto(CustomRole r)
    {
        List<string> perms;
        try
        {
            perms = JsonSerializer.Deserialize<List<string>>(r.PermissionsJson) ?? [];
        }
        catch (JsonException)
        {
            perms = [];
        }

        return new CustomRoleDto(RoleId.Format(r.Id), r.Name, perms, IsBuiltin: false, r.CreatedAt);
    }
}
