using System.Text;
using System.Text.Json;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using YamlDotNet.Core;

namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// Business logic for managing the shared vault configuration template per organization.
/// Handles permission checks, manifest validation, upsert with version increment, and
/// audit logging. Refs docs/SPECIFICATION.md :5654-5715.
/// </summary>
public sealed class VaultConfigService(
    AppDbContext db,
    RoleResolver roles,
    IAuditWriter audit,
    TimeProvider clock,
    IOptions<VaultConfigOptions> opts,
    ITeamVaultKeyProvider keyProvider,
    ILogger<VaultConfigService> logger)
{
    /// <summary>
    /// Fetches the vault-config row for the given organization.
    /// </summary>
    /// <returns>
    /// <c>(dto, None, null)</c> on success;
    /// <c>(null, NotFound, …)</c> when no row exists;
    /// <c>(null, PermissionDenied, …)</c> when user lacks <c>vault_config.view</c>.
    /// </returns>
    public async Task<(VaultConfigDto? dto, VaultConfigServiceError err, string? msg)>
        GetAsync(Guid actorId, Guid orgId, CancellationToken ct)
    {
        if (!await roles.HasPermissionAsync(actorId, orgId, Permissions.VaultConfigView, ct))
            return (null, VaultConfigServiceError.PermissionDenied, "Permission denied.");

        var row = await db.TeamVaults.FindAsync([orgId], ct);
        if (row is null)
            return (null, VaultConfigServiceError.NotFound, "Vault configuration not found.");

        var actorEmail = row.UpdatedBy.HasValue
            ? await ResolveEmailAsync(row.UpdatedBy.Value, ct)
            : null;

        // M18-009: return YAML from the ciphertext if the row has been encrypted.
        // During the backfill window (ciphertext is null) fall back to the plaintext TemplateYaml column.
        // After backfill is complete all rows have ciphertext; a decrypt failure then indicates key
        // rotation gone wrong or ciphertext corruption — surface it as DecryptionFailed (no fallback).
        string templateYaml = row.TemplateYaml;
        if (row.TemplateJsonCiphertext is { Length: > 0 } ciphertext && row.TemplateJsonKid is { } kid)
        {
            try
            {
                // The ciphertext stores the template YAML bytes (not the JSON form).
                var plainBytes = await keyProvider.DecryptAsync(ciphertext, kid, ct);
                templateYaml = Encoding.UTF8.GetString(plainBytes);
            }
            catch (TeamVaultDecryptException ex)
            {
                logger.LogError(ex,
                    "Failed to decrypt vault template for org {OrgId} (kid={Kid}) — returning error to caller",
                    orgId, kid);
                return (null, VaultConfigServiceError.DecryptionFailed,
                    "Vault template decryption failed (provider=team-vault). Contact your administrator.");
            }
        }

        var dto = new VaultConfigDto(templateYaml, row.Version, row.UpdatedAt, actorEmail);
        return (dto, VaultConfigServiceError.None, null);
    }

    /// <summary>
    /// Validates and upserts the vault-config template for the given organization.
    /// Increments version on success. Emits a <c>vault_config.upserted</c> audit event.
    /// </summary>
    /// <returns>
    /// <c>(dto, None, null, warnings)</c> on success;
    /// <c>(null, SuspiciousValue, msg, [...])</c> in reject mode with literal secrets;
    /// <c>(dto, None, null, [...])</c> in warn mode with literal secrets (warnings non-empty);
    /// <c>(null, InvalidYaml, msg, [])</c> on parse failure;
    /// <c>(null, PermissionDenied, msg, [])</c> when user lacks <c>vault_config.manage</c>.
    /// </returns>
    public async Task<(VaultConfigDto? dto, VaultConfigServiceError err, string? msg, IReadOnlyList<string> warnings)>
        UpsertAsync(Guid actorId, Guid orgId, string yamlBody, CancellationToken ct)
    {
        if (!await roles.HasPermissionAsync(actorId, orgId, Permissions.VaultConfigManage, ct))
            return (null, VaultConfigServiceError.PermissionDenied, "Permission denied.", []);

        if (string.IsNullOrWhiteSpace(yamlBody))
            return (null, VaultConfigServiceError.InvalidYaml, "Template body is required.", []);

        // Parse YAML → JSON (validate syntax)
        string json;
        try
        {
            json = VaultConfigYamlConverter.YamlToJson(yamlBody);
        }
        catch (YamlException ex)
        {
            return (null, VaultConfigServiceError.InvalidYaml,
                $"Invalid YAML: {ex.Message}", []);
        }

        // Run the manifest validator — dispose immediately after validation so pooled
        // ArrayPool<byte> buffers are returned promptly (JsonDocument is IDisposable).
        VaultValidationResult validationResult;
        using (var jsonDoc = JsonDocument.Parse(json))
        {
            validationResult = VaultManifestValidator.Validate(jsonDoc.RootElement);
        }

        IReadOnlyList<string> warnings = [];

        if (!validationResult.Ok)
        {
            if (opts.Value.IsRejectMode)
            {
                return (null, VaultConfigServiceError.SuspiciousValue,
                    $"Template contains suspicious values at: {string.Join(", ", validationResult.OffendingPaths)}",
                    validationResult.OffendingPaths);
            }

            // Warn mode — log and continue
            logger.LogWarning(
                "Vault-config template for org {OrgId} contains suspicious values at paths: {Paths}",
                orgId,
                string.Join(", ", validationResult.OffendingPaths));
            warnings = validationResult.OffendingPaths;
        }

        var now = clock.GetUtcNow().UtcDateTime;

        // M18-009: Encrypt the YAML template before persisting.
        // Validator has already run above against the cleartext — encryption is additive.
        var yamlBytes = Encoding.UTF8.GetBytes(yamlBody);
        var encResult = await keyProvider.EncryptAsync(yamlBytes, ct);

        // Upsert: update if exists, insert if not
        var existing = await db.TeamVaults.FindAsync([orgId], ct);
        long newVersion;

        if (existing is null)
        {
            newVersion = 1;
            db.TeamVaults.Add(new TeamVault
            {
                OrgId = orgId,
                TemplateYaml = yamlBody,
                TemplateJson = json,
                TemplateJsonCiphertext = encResult.Ciphertext,
                TemplateJsonKid = encResult.Kid,
                Version = newVersion,
                CreatedAt = now,
                CreatedBy = actorId,
                UpdatedAt = now,
                UpdatedBy = actorId,
            });
        }
        else
        {
            newVersion = existing.Version + 1;
            existing.TemplateYaml = yamlBody;
            existing.TemplateJson = json;
            existing.TemplateJsonCiphertext = encResult.Ciphertext;
            existing.TemplateJsonKid = encResult.Kid;
            existing.Version = newVersion;
            existing.UpdatedAt = now;
            existing.UpdatedBy = actorId;
        }

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: actorId,
            EventType: "vault_config.upserted",
            Payload: new { version = newVersion, suspicious_warnings_count = warnings.Count }));

        await db.SaveChangesAsync(ct);

        var actorEmail = await ResolveEmailAsync(actorId, ct);
        var dto = new VaultConfigDto(yamlBody, newVersion, now, actorEmail);
        return (dto, VaultConfigServiceError.None, null, warnings);
    }

    /// <summary>
    /// Deletes the vault-config row for the given organization.
    /// Emits a <c>vault_config.deleted</c> audit event on success.
    /// </summary>
    /// <returns>
    /// <c>None</c> on success;
    /// <c>NotFound</c> when no row exists;
    /// <c>PermissionDenied</c> when user lacks <c>vault_config.manage</c>.
    /// </returns>
    public async Task<VaultConfigServiceError> DeleteAsync(Guid actorId, Guid orgId, CancellationToken ct)
    {
        if (!await roles.HasPermissionAsync(actorId, orgId, Permissions.VaultConfigManage, ct))
            return VaultConfigServiceError.PermissionDenied;

        var existing = await db.TeamVaults.FindAsync([orgId], ct);
        if (existing is null)
            return VaultConfigServiceError.NotFound;

        var previousVersion = existing.Version;
        db.TeamVaults.Remove(existing);

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: actorId,
            EventType: "vault_config.deleted",
            Payload: new { previous_version = previousVersion }));

        await db.SaveChangesAsync(ct);
        return VaultConfigServiceError.None;
    }

    private async Task<string?> ResolveEmailAsync(Guid userId, CancellationToken ct)
    {
        var user = await db.Users.FindAsync([userId], ct);
        return user?.Email;
    }
}
