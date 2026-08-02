// GitLab integrations endpoints — GET list, POST create, DELETE soft-delete.
// Implements M16-016 replacing the M16-013 stubs.
// Refs docs/SPECIFICATION.md:9177-9195 (Per-Org GitLab Configuration).
using System.Text;
using System.Text.Json.Serialization;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.GitLab.Installations;

/// <summary>Response body for GET /api/v1/integrations/gitlab.</summary>
public sealed record GitLabInstallationListResponse(IReadOnlyList<GitLabInstallationListItem> Installations);

/// <summary>Summary of a single GitLab installation.</summary>
public sealed record GitLabInstallationListItem(
    Guid Id,
    long ProjectId,
    string ProjectPath,
    [property: JsonPropertyName("gitlab_base_url")] string GitLabBaseUrl,
    DateTimeOffset CreatedAt,
    DateTimeOffset? AccessTokenRevokedAt,
    DateTimeOffset? LastStatusPostAt = null);

/// <summary>Registers the /api/v1/integrations/gitlab endpoint group.</summary>
public static class GitLabIntegrationsEndpoints
{
    private const string FeatureCode = "gitlab_integrations";

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    /// <summary>Maps all GitLab integration endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapGitLabIntegrationsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/integrations/gitlab")
            .WithTags("GitLab Integrations");

        group.MapGet("", ListInstallations)
            .RequireAuthorization()
            .WithName("ListGitLabIntegrations")
            .Produces<GitLabInstallationListResponse>()
            .ProducesProblem(StatusCodes.Status402PaymentRequired)
            .Produces(StatusCodes.Status401Unauthorized);

        group.MapPost("", CreateInstallation)
            .RequireAuthorization()
            .WithName("CreateGitLabIntegration")
            .Accepts<CreateGitLabIntegrationRequest>("application/json")
            .Produces<GitLabInstallationListItem>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status409Conflict)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        group.MapDelete("{id:guid}", DeleteInstallation)
            .RequireAuthorization()
            .WithName("DeleteGitLabIntegration")
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound)
            .ProducesProblem(StatusCodes.Status402PaymentRequired);

        return app;
    }

    // ── GET /api/v1/integrations/gitlab ──────────────────────────────────────

    private static async Task<IResult> ListInstallations(
        HttpContext http,
        [FromQuery(Name = "org_id")] string? orgIdParam,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        var orgResult = await ResolveOrgAsync(db, userId.Value, orgIdParam, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != VaultConfigError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        // Query installations (non-deleted) with most-recent pr_check post time.
        var rows = await db.GitLabInstallations
            .Where(i => i.OrgId == orgId && i.DeletedAt == null)
            .OrderBy(i => i.CreatedAt)
            .ToListAsync(ct);

        // Fetch max posted_at per installation_id from pr_checks.
        var instIds = rows.Select(r => (Guid?)r.Id).ToList();
        var lastPosts = await db.PrChecks
            .Where(c => c.GitLabInstallationId != null && instIds.Contains(c.GitLabInstallationId))
            .GroupBy(c => c.GitLabInstallationId)
            .Select(g => new { InstId = g.Key, LastPost = g.Max(c => c.PostedAt) })
            .ToListAsync(ct);

        var lastPostMap = lastPosts.ToDictionary(x => x.InstId!.Value, x => x.LastPost);

        var items = rows.Select(r =>
        {
            lastPostMap.TryGetValue(r.Id, out var lastPost);
            return new GitLabInstallationListItem(
                r.Id,
                r.ProjectId,
                r.ProjectPath,
                r.GitLabBaseUrl,
                new DateTimeOffset(r.CreatedAt, TimeSpan.Zero),
                r.AccessTokenRevokedAt.HasValue
                    ? new DateTimeOffset(r.AccessTokenRevokedAt.Value, TimeSpan.Zero)
                    : null,
                lastPost.HasValue ? new DateTimeOffset(lastPost.Value, TimeSpan.Zero) : null);
        }).ToList();

        return HttpResults.Ok(new GitLabInstallationListResponse(items));
    }

    // ── POST /api/v1/integrations/gitlab ─────────────────────────────────────

    private static async Task<IResult> CreateInstallation(
        HttpContext http,
        [FromQuery(Name = "org_id")] string? orgIdParam,
        CurrentUserAccessor users,
        ITierGate tierGate,
        IGitLabProjectLookup lookup,
        IGitLabKeyProvider keyProvider,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        // Parse request body.
        CreateGitLabIntegrationRequest? body;
        try
        {
            body = await http.Request.ReadFromJsonAsync<CreateGitLabIntegrationRequest>(ct);
        }
        catch
        {
            return BadRequest400("invalid_request", "Request body is not valid JSON.");
        }

        if (body is null || string.IsNullOrWhiteSpace(body.ProjectPath))
            return BadRequest400("invalid_request", "project_path is required.");
        if (string.IsNullOrWhiteSpace(body.AccessToken))
            return BadRequest400("invalid_request", "access_token is required.");

        var baseUrl = string.IsNullOrWhiteSpace(body.GitLabBaseUrl)
            ? "https://gitlab.com"
            : body.GitLabBaseUrl;

        var orgResult = await ResolveOrgAsync(db, userId.Value, orgIdParam, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != VaultConfigError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        // Resolve project_id from GitLab.
        var lookupResult = await lookup.LookupAsync(baseUrl, body.ProjectPath, body.AccessToken, body.GitLabCaBundle, ct);
        switch (lookupResult.Status)
        {
            case GitLabProjectLookupStatus.NotFound:
                return BadRequest400("project_not_found",
                    "Project not found or PAT lacks access. Verify the path and token scopes.");
            case GitLabProjectLookupStatus.Unauthorized:
                return BadRequest400("pat_unauthorized",
                    "The access token was rejected by GitLab (401). Check the token is valid and has not been revoked.");
            case GitLabProjectLookupStatus.InsecureBaseUrl:
                return BadRequest400("insecure_base_url",
                    "The gitlab_base_url uses http:// which is not permitted. Use https://.");
            case GitLabProjectLookupStatus.InvalidBaseUrl:
                return BadRequest400("invalid_base_url",
                    "gitlab_base_url must be a valid absolute URL (e.g. https://gitlab.com).");
            case GitLabProjectLookupStatus.Unreachable:
                return HttpResults.Json(
                    new ErrorResponse("gitlab_unreachable",
                        "Could not reach the GitLab instance. Verify the base URL and network connectivity."),
                    statusCode: StatusCodes.Status502BadGateway);
            case GitLabProjectLookupStatus.InvalidResponse:
                return HttpResults.Json(
                    new ErrorResponse("gitlab_invalid_response",
                        "GitLab returned an unexpected response. Try again or contact support."),
                    statusCode: StatusCodes.Status502BadGateway);
        }

        // Use the GitLab-normalized path (resolvedPath) for display; use the numeric
        // project_id for the duplicate pre-check so the guard aligns with the DB partial
        // unique index on (org_id, project_id, gitlab_base_url).
        var resolvedPath = lookupResult.ResolvedPath ?? body.ProjectPath;
        var resolvedProjectId = lookupResult.ProjectId!.Value;

        // Check for duplicate (same org + numeric project_id + base_url, not soft-deleted).
        // Using project_id matches the DB index invariant exactly, preventing bypasses via
        // renamed or aliased project paths that resolve to the same project_id.
        var duplicate = await db.GitLabInstallations.AnyAsync(
            i => i.OrgId == orgId
              && i.ProjectId == resolvedProjectId
              && i.GitLabBaseUrl == baseUrl
              && i.DeletedAt == null,
            ct);
        if (duplicate)
            return HttpResults.Json(
                new ErrorResponse("duplicate_integration",
                    "An active integration for this project path already exists."),
                statusCode: StatusCodes.Status409Conflict);

        // Encrypt PAT.
        var patBytes = Encoding.UTF8.GetBytes(body.AccessToken);
        var encrypted = await keyProvider.EncryptAsync(patBytes, ct);

        var now = DateTime.UtcNow;
        var row = new Data.Entities.GitLabInstallation
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            ProjectId = resolvedProjectId,
            ProjectPath = resolvedPath,
            GitLabBaseUrl = baseUrl,
            GitLabCaBundle = body.GitLabCaBundle,
            AccessTokenCiphertext = encrypted.Ciphertext,
            AccessTokenKid = encrypted.Kid,
            CreatedAt = now,
            UpdatedAt = now,
        };
        db.GitLabInstallations.Add(row);
        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateException ex) when (IsUniqueConstraintViolation(ex))
        {
            // Race: two concurrent identical POSTs can both pass the AnyAsync guard before
            // either inserts; the DB partial unique index on (org_id, project_id, gitlab_base_url)
            // then rejects the second writer with a constraint violation. Surface as 409.
            return HttpResults.Json(
                new ErrorResponse("duplicate_integration",
                    "An active integration for this project path already exists."),
                statusCode: StatusCodes.Status409Conflict);
        }

        var item = new GitLabInstallationListItem(
            row.Id,
            row.ProjectId,
            row.ProjectPath,
            row.GitLabBaseUrl,
            new DateTimeOffset(row.CreatedAt, TimeSpan.Zero),
            null,
            null);

        return HttpResults.Json(item, statusCode: StatusCodes.Status201Created);
    }

    // ── DELETE /api/v1/integrations/gitlab/{id} ───────────────────────────────

    private static async Task<IResult> DeleteInstallation(
        Guid id,
        HttpContext http,
        [FromQuery(Name = "org_id")] string? orgIdParam,
        CurrentUserAccessor users,
        ITierGate tierGate,
        AppDbContext db,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null) return Unauthorized401;

        var orgResult = await ResolveOrgAsync(db, userId.Value, orgIdParam, ct);
        if (orgResult.error is not null) return orgResult.error;
        var orgId = orgResult.orgId!.Value;

        var gateErr = await VaultConfigTierGate.EnsureTeamOrAboveAsync(tierGate, orgId, ct);
        if (gateErr != VaultConfigError.None)
            return await MapGateErrorAsync(http, gateErr, db, orgId, ct);

        var row = await db.GitLabInstallations
            .FirstOrDefaultAsync(i => i.Id == id && i.OrgId == orgId && i.DeletedAt == null, ct);

        if (row is null)
            return HttpResults.Json(
                new ErrorResponse("not_found", "Integration not found or already deleted."),
                statusCode: StatusCodes.Status404NotFound);

        var now = DateTime.UtcNow;
        row.DeletedAt = now;
        row.UpdatedAt = now;
        await db.SaveChangesAsync(ct);

        return HttpResults.NoContent();
    }

    // ── private helpers ───────────────────────────────────────────────────────

    /// <summary>
    /// Resolves the caller's org from an explicit <paramref name="orgIdParam"/> query parameter or,
    /// when absent, auto-resolves to the single org the user belongs to.
    /// Returns 400 <c>ambiguous_org_id</c> when the user belongs to multiple orgs without specifying one,
    /// and 403 <c>no_org_membership</c> when the user belongs to no org.
    /// </summary>
    private static async Task<(Guid? orgId, IResult? error)> ResolveOrgAsync(
        AppDbContext db, Guid userId, string? orgIdParam, CancellationToken ct)
    {
        if (orgIdParam is not null)
        {
            // Accept both raw GUID and "org_<guid>" format.
            var hex = orgIdParam.StartsWith("org_", StringComparison.OrdinalIgnoreCase)
                ? orgIdParam[4..]
                : orgIdParam;

            if (!Guid.TryParse(hex, out var parsedOrgId))
                return (null, HttpResults.Json(
                    new ErrorResponse("invalid_org_id", "The org_id parameter is not a valid org ID."),
                    statusCode: StatusCodes.Status400BadRequest));

            // Verify the caller is a member of the specified org.
            var isMember = await db.OrganizationMembers
                .AnyAsync(m => m.OrgId == parsedOrgId && m.UserId == userId, ct);

            if (!isMember)
                return (null, HttpResults.Json(
                    new ErrorResponse("organization_not_found", "Organization not found or user is not a member."),
                    statusCode: StatusCodes.Status403Forbidden));

            return (parsedOrgId, null);
        }

        // Auto-resolve: user must be in exactly one org.
        var memberships = await db.OrganizationMembers
            .Where(m => m.UserId == userId)
            .Select(m => m.OrgId)
            .ToListAsync(ct);

        if (memberships.Count == 0)
            return (null, HttpResults.Json(
                new ErrorResponse("no_org_membership",
                    "The authenticated user is not a member of any organization."),
                statusCode: StatusCodes.Status403Forbidden));

        if (memberships.Count > 1)
            return (null, HttpResults.Json(
                new ErrorResponse("ambiguous_org_id",
                    "This user belongs to multiple organizations. Specify ?org_id=."),
                statusCode: StatusCodes.Status400BadRequest));

        return (memberships[0], null);
    }

    /// <summary>Maps a tier-gate denial to the appropriate RFC 7807 HTTP response.</summary>
    private static async Task<IResult> MapGateErrorAsync(
        HttpContext http,
        VaultConfigError err,
        AppDbContext db,
        Guid orgId,
        CancellationToken ct)
    {
        if (err == VaultConfigError.OrgNotFound)
            return TierGateProblemFactory.AuthenticatedOrgNotFound(http);

        return await TierGateProblemFactory.AuthenticatedTierIneligibleAsync(
            http, db, orgId, VaultConfigTierGate.RequiredMinimum, FeatureCode, ct);
    }

    private static IResult BadRequest400(string code, string description) =>
        HttpResults.Json(
            new ErrorResponse(code, description),
            statusCode: StatusCodes.Status400BadRequest);

    /// <summary>
    /// Returns <see langword="true"/> when a <see cref="DbUpdateException"/> is caused by a
    /// UNIQUE constraint violation. Mirrors the guard used in <c>OrganizationService</c> and
    /// <c>GithubInstallationsService</c>; non-constraint DB errors (connection failures, lock
    /// timeouts) are left to propagate so the global error handler logs and returns a 500.
    /// </summary>
    private static bool IsUniqueConstraintViolation(DbUpdateException ex) =>
        ex.InnerException?.Message.Contains("UNIQUE constraint failed", StringComparison.OrdinalIgnoreCase) == true
        || ex.InnerException?.Message.Contains("UNIQUE", StringComparison.OrdinalIgnoreCase) == true;
}
