using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.VaultConfig;

/// <summary>Registers the vault-config endpoints onto the route builder.</summary>
public static class VaultConfigEndpoints
{
    /// <summary>Maps GET/PUT/DELETE /api/v1/organizations/{orgId}/vault-config onto the given builder.</summary>
    public static IEndpointRouteBuilder MapVaultConfigEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}/vault-config")
            .RequireAuthorization()
            .WithTags("VaultConfig");

        group.MapGet("", GetVaultConfig)
            .WithName("GetVaultConfig")
            .Produces<VaultConfigDto>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status304NotModified)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapPut("", PutVaultConfig)
            .WithName("PutVaultConfig")
            .Produces<VaultConfigDto>(StatusCodes.Status200OK)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .ProducesProblem(StatusCodes.Status422UnprocessableEntity);

        group.MapDelete("", DeleteVaultConfig)
            .WithName("DeleteVaultConfig")
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> GetVaultConfig(
        string orgId,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        VaultConfigService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == Internal.TierGates.VaultConfigError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, VaultConfigTierGate.RequiredMinimum, "vault_config", ct);
        if (gateErr == Internal.TierGates.VaultConfigError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var (dto, err, _) = await svc.GetAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            VaultConfigServiceError.None => BuildGetResponse(http, dto!),
            VaultConfigServiceError.NotFound => HttpResults.Json(
                new ErrorResponse("vault_config_not_found", "Vault configuration not found."),
                statusCode: StatusCodes.Status404NotFound),
            VaultConfigServiceError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            VaultConfigServiceError.DecryptionFailed =>
                HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static IResult BuildGetResponse(HttpContext http, VaultConfigDto dto)
    {
        var etag = $"\"{dto.Version}\"";
        var ifNoneMatch = http.Request.Headers.IfNoneMatch.ToString();

        // 304 when If-None-Match matches the current version (strong ETag comparison)
        if (!string.IsNullOrEmpty(ifNoneMatch))
        {
            // Strip quotes for comparison; handle wildcard *
            var clientTag = ifNoneMatch.Trim('"');
            var serverTag = dto.Version.ToString();

            if (clientTag == "*" || clientTag == serverTag)
            {
                http.Response.Headers.ETag = etag;
                return HttpResults.StatusCode(StatusCodes.Status304NotModified);
            }
        }

        http.Response.Headers.ETag = etag;
        return HttpResults.Ok(new
        {
            template = dto.Template,
            version = dto.Version,
            updated_at = dto.UpdatedAt,
            updated_by_email = dto.UpdatedByEmail,
        });
    }

    private static async Task<IResult> PutVaultConfig(
        string orgId,
        HttpContext http,
        HttpRequest request,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        VaultConfigService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == Internal.TierGates.VaultConfigError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, VaultConfigTierGate.RequiredMinimum, "vault_config", ct);
        if (gateErr == Internal.TierGates.VaultConfigError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        // Read raw body as YAML text — any Content-Type with text body accepted
        using var reader = new System.IO.StreamReader(request.Body);
        var yamlBody = await reader.ReadToEndAsync(ct);

        var (dto, err, msg, warnings) = await svc.UpsertAsync(userId.Value, orgGuid, yamlBody, ct);

        if (err == VaultConfigServiceError.None)
        {
            http.Response.Headers.ETag = $"\"{dto!.Version}\"";
            return HttpResults.Ok(new
            {
                template = dto.Template,
                version = dto.Version,
                updated_at = dto.UpdatedAt,
                updated_by_email = dto.UpdatedByEmail,
                warnings = warnings.Count > 0 ? warnings : null,
            });
        }

        return err switch
        {
            VaultConfigServiceError.SuspiciousValue =>
                VaultConfigProblem.SuspiciousValue(http, warnings),
            VaultConfigServiceError.InvalidYaml => HttpResults.Json(
                new ErrorResponse("invalid_yaml", msg ?? "Invalid YAML."),
                statusCode: StatusCodes.Status400BadRequest),
            VaultConfigServiceError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> DeleteVaultConfig(
        string orgId,
        HttpContext http,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        VaultConfigService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgGuid, ct);
        if (gateErr == Internal.TierGates.VaultConfigError.TierIneligible)
            return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
                http, db, orgGuid, VaultConfigTierGate.RequiredMinimum, "vault_config", ct);
        if (gateErr == Internal.TierGates.VaultConfigError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        var err = await svc.DeleteAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            VaultConfigServiceError.None => HttpResults.NoContent(),
            VaultConfigServiceError.NotFound => HttpResults.Json(
                new ErrorResponse("vault_config_not_found", "Vault configuration not found."),
                statusCode: StatusCodes.Status404NotFound),
            VaultConfigServiceError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }
}
