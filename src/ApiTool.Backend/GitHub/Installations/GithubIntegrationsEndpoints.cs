// Refs docs/SPECIFICATION.md:8413-8416 (install-url + callback endpoints).
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.GitHub.Installations;

/// <summary>Response from GET /api/v1/integrations/github/install-url.</summary>
public sealed record InstallUrlResponse(string InstallUrl, DateTimeOffset StateExpiresAt);

/// <summary>Registers the /api/v1/integrations/github endpoint group.</summary>
public static class GithubIntegrationsEndpoints
{
    /// <summary>Maps all GitHub integration endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapGithubIntegrationsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/integrations/github")
            .WithTags("GitHub Integrations");

        group.MapGet("install-url", GetInstallUrl)
            .RequireAuthorization()
            .WithName("GetGithubInstallUrl")
            .Produces<InstallUrlResponse>()
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status403Forbidden)
            .Produces(StatusCodes.Status404NotFound);

        group.MapGet("callback", HandleCallback)
            .WithName("HandleGithubCallback")
            .Produces(StatusCodes.Status302Found)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status409Conflict);

        group.MapPost("claim", StubClaim)
            .RequireAuthorization()
            .WithName("ClaimGithubInstallation")
            .Produces(StatusCodes.Status501NotImplemented);

        return app;
    }

    // ── GET /install-url ──────────────────────────────────────────────────────────

    private static async Task<IResult> GetInstallUrl(
        [FromQuery(Name = "org_id")] string? orgIdParam,
        CurrentUserAccessor users,
        AppDbContext db,
        TimeProvider clock,
        IOptions<GitHubAppOptions> ghOptions,
        IOptions<AppOptions> appOptions,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Json(
                new ErrorResponse("unauthorized", "Authentication is required."),
                statusCode: StatusCodes.Status401Unauthorized);

        // Resolve org_id
        Guid orgId;
        if (orgIdParam is not null)
        {
            // Parse org_NNN format
            var hex = orgIdParam.StartsWith("org_", StringComparison.OrdinalIgnoreCase)
                ? orgIdParam[4..]
                : orgIdParam;
            if (!Guid.TryParse(hex, out orgId))
                return HttpResults.Json(
                    new ErrorResponse("invalid_org_id", "The org_id parameter is not a valid org ID."),
                    statusCode: StatusCodes.Status400BadRequest);
        }
        else
        {
            // Auto-resolve: user must be in exactly one org
            var memberships = await db.OrganizationMembers
                .Where(m => m.UserId == userId.Value)
                .Select(m => m.OrgId)
                .ToListAsync(ct);

            if (memberships.Count == 0)
                return HttpResults.Json(
                    new ErrorResponse("organization_not_found", "No organization found for this user."),
                    statusCode: StatusCodes.Status404NotFound);

            if (memberships.Count > 1)
                return HttpResults.Json(
                    new ErrorResponse("ambiguous_org_id", "This user belongs to multiple organizations. Specify ?org_id=."),
                    statusCode: StatusCodes.Status400BadRequest);

            orgId = memberships[0];
        }

        // Check membership: must be Owner or Admin
        var membership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId.Value, ct);

        if (membership is null)
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found or user is not a member."),
                statusCode: StatusCodes.Status404NotFound);

        if (membership.Role == OrgRole.Member)
            return HttpResults.Json(
                new ErrorResponse("forbidden", "Only admin or owner can initiate a GitHub App install."),
                statusCode: StatusCodes.Status403Forbidden);

        // Mint the signed state token (15-minute expiry)
        var expiresAt = clock.GetUtcNow().AddMinutes(15);
        var nonce = Convert.ToHexString(System.Security.Cryptography.RandomNumberGenerator.GetBytes(8));
        var payload = new GithubInstallationStatePayload(orgId, userId.Value, expiresAt, nonce);

        var opts = ghOptions.Value;
        if (string.IsNullOrEmpty(opts.StateSigningKey))
            return HttpResults.Json(
                new ErrorResponse("configuration_error",
                    "ApiTool:GitHubApp:StateSigningKey is not configured."),
                statusCode: StatusCodes.Status500InternalServerError);

        var stateToken = GithubInstallationStateToken.Mint(payload, opts.StateSigningKey);
        var installUrl = $"https://github.com/apps/{opts.Slug}/installations/new?state={stateToken}";

        return HttpResults.Ok(new InstallUrlResponse(installUrl, expiresAt));
    }

    // ── GET /callback ─────────────────────────────────────────────────────────────

    private static async Task<IResult> HandleCallback(
        [FromQuery(Name = "installation_id")] long? installationId,
        [FromQuery] string? state,
        AppDbContext db,
        TimeProvider clock,
        IOptions<GitHubAppOptions> ghOptions,
        IOptions<AppOptions> appOptions,
        GithubInstallationsService installationsService,
        CancellationToken ct)
    {
        // Validate state token
        if (string.IsNullOrEmpty(state) || installationId is null)
            return HttpResults.Json(
                new ErrorResponse("invalid_state", "Missing state or installation_id."),
                statusCode: StatusCodes.Status401Unauthorized);

        var opts = ghOptions.Value;
        if (string.IsNullOrEmpty(opts.StateSigningKey))
            return HttpResults.Json(
                new ErrorResponse("configuration_error",
                    "ApiTool:GitHubApp:StateSigningKey is not configured."),
                statusCode: StatusCodes.Status500InternalServerError);

        var payload = GithubInstallationStateToken.TryVerify(state, opts.StateSigningKey, clock.GetUtcNow());
        if (payload is null)
            return HttpResults.Json(
                new ErrorResponse("invalid_state", "The state token is invalid or has expired."),
                statusCode: StatusCodes.Status401Unauthorized);

        // Verify org still exists
        var orgExists = await db.Organizations.AnyAsync(o => o.Id == payload.OrgId, ct);
        if (!orgExists)
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "The organization no longer exists."),
                statusCode: StatusCodes.Status404NotFound);

        // Claim the installation
        var claimErr = await installationsService.ClaimAsync(
            installationId.Value, payload.OrgId, opts.AppId, ct);

        if (claimErr == InstallClaimError.AlreadyLinked)
            return HttpResults.Json(
                new ErrorResponse("install_already_linked",
                    "This organization already has an active GitHub installation."),
                statusCode: StatusCodes.Status409Conflict);

        // Redirect to web app billing page
        var redirectUrl = $"{appOptions.Value.WebAppUrl.TrimEnd('/')}/billing?installed=true";
        return HttpResults.Redirect(redirectUrl);
    }

    // ── POST /claim (stub — M14-018) ──────────────────────────────────────────────

    private static IResult StubClaim() =>
        HttpResults.Json(
            new ErrorResponse("not_implemented",
                "Webhook-first claim verification requires M14-018 (GitHub API client). Not yet implemented."),
            statusCode: StatusCodes.Status501NotImplemented);
}
