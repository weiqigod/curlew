using System.Linq.Expressions;
using System.Reflection;
using System.Text.Json;
using System.Text.Json.Nodes;
using ApiTool.Backend.Data;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Assembles the per-user GDPR export bundle by iterating every <c>InExport</c> entry in the
/// <see cref="GdprBundleManifest"/> and querying the database for rows belonging to the given user.
/// The bundle is manifest-driven: no entity names are hardcoded here.
/// </summary>
public static class UserExportBundleAssembler
{
    private static readonly JsonSerializerOptions s_jsonOpts = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
    };

    /// <summary>
    /// Assembles the export bundle for <paramref name="userId"/> using the manifest's
    /// <c>InExport</c> entries. Returns a <see cref="JsonObject"/> with the canonical shape:
    /// <code>
    /// {
    ///   "user_id": "...",
    ///   "generated_at": "...",
    ///   "schema_version": 1,
    ///   "tables": { "users": [...], "refresh_tokens": [...], ... }
    /// }
    /// </code>
    /// </summary>
    public static async Task<JsonObject> AssembleAsync(
        AppDbContext db,
        GdprBundleManifest manifest,
        Guid userId,
        DateTime generatedAt,
        CancellationToken ct)
    {
        ArgumentNullException.ThrowIfNull(db);
        ArgumentNullException.ThrowIfNull(manifest);

        var tables = new JsonObject();

        foreach (var entry in manifest.InExport)
        {
            var rows = await QueryTableAsync(db, entry, userId, ct);
            tables[entry.TableName] = rows;
        }

        return new JsonObject
        {
            ["user_id"] = userId.ToString(),
            ["generated_at"] = generatedAt.ToString("O"),
            ["schema_version"] = 1,
            ["tables"] = tables,
        };
    }

    // Queries one InExport table for rows attributed to userId using the manifest's
    // UserIdColumnName. Falls back to a safe empty array if the column name is null.
    private static async Task<JsonArray> QueryTableAsync(
        AppDbContext db,
        GdprManifestEntry entry,
        Guid userId,
        CancellationToken ct)
    {
        if (string.IsNullOrEmpty(entry.UserIdColumnName))
            return [];

        // Build typed query dynamically via reflection on EF's Set<TEntity>() method.
        var method = typeof(UserExportBundleAssembler)
            .GetMethod(nameof(QueryTypedTableAsync), BindingFlags.NonPublic | BindingFlags.Static)!
            .MakeGenericMethod(entry.EntityType);

        var task = (Task<JsonArray>)method.Invoke(null, [db, entry.UserIdColumnName, userId, ct])!;
        return await task;
    }

    private static async Task<JsonArray> QueryTypedTableAsync<TEntity>(
        AppDbContext db,
        string userIdColumnName,
        Guid userId,
        CancellationToken ct)
        where TEntity : class
    {
        // Build a lambda: entity => entity.<UserIdColumnName> == userId
        // The property may be Guid? (nullable) after the M18-006 column widening;
        // lift the constant to Guid? so Expression.Equal resolves without error.
        var param = Expression.Parameter(typeof(TEntity), "e");
        var prop = Expression.Property(param, userIdColumnName);
        var propType = prop.Type;
        Expression constant = propType == typeof(Guid?)
            ? Expression.Constant((Guid?)userId, typeof(Guid?))
            : Expression.Constant(userId, typeof(Guid));
        var equals = Expression.Equal(prop, constant);
        var predicate = Expression.Lambda<Func<TEntity, bool>>(equals, param);

        var rows = await db.Set<TEntity>()
            .AsNoTracking()
            .Where(predicate)
            .ToListAsync(ct);

        var array = new JsonArray();
        foreach (var row in rows)
        {
            // Serialize to JSON and deserialize to JsonObject to strip type info.
            var json = JsonSerializer.Serialize(row, s_jsonOpts);
            var node = JsonNode.Parse(json)!.AsObject();

            // M18-004 risk: filter out sensitive columns (PasswordHash, TokenHash).
            // Remove properties that should never appear in a user export.
            foreach (var sensitive in SensitivePropertyNames)
                node.Remove(sensitive);

            array.Add(node);
        }

        return array;
    }

    // Columns filtered from every exported row regardless of entity.
    // These hold secrets at rest; serializing them in the export bundle would be a data leak.
    private static readonly HashSet<string> SensitivePropertyNames =
    [
        "password_hash",
        "token_hash",
    ];
}
