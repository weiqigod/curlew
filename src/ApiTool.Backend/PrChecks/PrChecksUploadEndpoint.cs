// Refs docs/SPECIFICATION.md:8429-8550 (POST /api/v1/pr-checks, v4.2.1 shape).
// M16-014: adds provider discriminator dispatch (github | gitlab).
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.Results;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Handler for <c>POST /api/v1/pr-checks</c>.
/// Creates a <c>pr_checks</c> row and synchronously calls <see cref="CheckRunPoster"/>
/// to post the check run to GitHub. Resolves the organisation from the bearer token's
/// first membership; multi-org disambiguation is deferred to a future milestone.
/// </summary>
internal static class PrChecksUploadEndpoint
{
    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    internal static async Task<IResult> HandleAsync(
        HttpRequest request,
        HttpContext http,
        CurrentUserAccessor users,
        AppDbContext db,
        ICheckRunPoster githubPoster,
        IGitLabCheckPoster gitlabPoster,
        TimeProvider clock,
        ILoggerFactory loggerFactory,
        CancellationToken ct)
    {
        var log = loggerFactory.CreateLogger("ApiTool.Backend.PrChecks.PrChecksUploadEndpoint");

        // Step 1: Read raw body (needed for token-leak scan before deserialisation)
        request.EnableBuffering();
        var rawBytes = await ReadBodyAsync(request, ct);
        var rawBody = Encoding.UTF8.GetString(rawBytes);

        // Step 2: Token leak check — log security incident per spec :8643
        if (TokenLeakDetector.ContainsTokenLeak(rawBody))
        {
            log.LogWarning(
                new EventId(8643, "PRCHECK_TOKEN_LEAK_DETECTED"),
                "Security incident: GitHub installation token detected in request body. " +
                "security_incident=true TraceId={TraceId}",
                http.TraceIdentifier);
            return PrChecksProblem.TokenLeakDetected(http);
        }

        // Step 3: Deserialise
        PrCheckUploadRequest? body;
        try
        {
            body = JsonSerializer.Deserialize<PrCheckUploadRequest>(rawBody, SnakeCaseOptions);
        }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        if (body is null)
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is required."),
                statusCode: StatusCodes.Status400BadRequest);

        // Step 4: Resolve user
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        // Step 4 (cont): Resolve org from unique membership
        var memberships = await db.OrganizationMembers
            .Where(m => m.UserId == userId.Value)
            .Select(m => m.OrgId)
            .ToListAsync(ct);

        if (memberships.Count == 0)
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "No organisation found for this user."),
                statusCode: StatusCodes.Status404NotFound);

        // If multi-org, require explicit ?org_id parameter (not in scope for M14-018).
        // Log a warning so production misrouting is visible in logs.
        if (memberships.Count > 1)
        {
            log.LogWarning(
                "User {UserId} belongs to {Count} orgs; selecting first for pr-check. " +
                "Multi-org disambiguation is deferred to a future milestone.",
                userId.Value, memberships.Count);
        }
        var orgId = memberships[0];

        // Step 5: Validate state
        if (!PrCheckConclusionMapper.TryMap(body.State, out _))
            return PrChecksProblem.InvalidState(http,
                $"state '{body.State}' is not valid. Must be one of: {string.Join(", ", PrCheckConclusionMapper.ValidStates)}.");

        // Validate head_sha
        if (string.IsNullOrWhiteSpace(body.HeadSha) || body.HeadSha.Length != 40)
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "head_sha must be a 40-character hex string."),
                statusCode: StatusCodes.Status400BadRequest);

        if (string.IsNullOrWhiteSpace(body.Repo))
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "repo is required."),
                statusCode: StatusCodes.Status400BadRequest);

        if (body.Pr is null || body.Pr.Value <= 0)
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "pr must be a positive integer."),
                statusCode: StatusCodes.Status400BadRequest);

        // Step 5b: Resolve and validate provider (Decision J — absent defaults to 'github')
        var provider = string.IsNullOrWhiteSpace(body.Provider)
            ? "github"
            : body.Provider.Trim().ToLowerInvariant();
        if (provider is not ("github" or "gitlab"))
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "provider must be 'github' or 'gitlab'."),
                statusCode: StatusCodes.Status400BadRequest);

        // Step 6: Resolve optional GitLab installation FK
        Guid? glInstallationId = null;
        if (provider == "gitlab")
        {
            var gl = await db.GitLabInstallations
                .Where(i => i.OrgId == orgId && i.DeletedAt == null)
                .OrderByDescending(i => i.CreatedAt)
                .FirstOrDefaultAsync(ct);
            if (gl is null)
                return PrChecksProblem.GitLabNoInstallation(http);
            glInstallationId = gl.Id;
        }

        // Step 6b: Insert PrCheck row
        var now = clock.GetUtcNow().UtcDateTime;
        var check = new PrCheck
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Repo = body.Repo,
            Pr = body.Pr.Value,
            State = body.State!.ToLowerInvariant(),
            HeadSha = body.HeadSha,
            ExternalId = Guid.NewGuid(),
            Status = "pending",
            Provider = provider,
            GitLabInstallationId = glInstallationId,
            CreatedAt = now,
            DetailsUrl = body.DetailsUrl,
            OutputSummary = body.Output?.Summary,
            OutputText = body.Output?.Text,
            AnnotationsJson = body.Output?.Annotations is { Count: > 0 }
                ? JsonSerializer.Serialize(body.Output.Annotations.Take(50))
                : null,
        };

        db.PrChecks.Add(check);
        await db.SaveChangesAsync(ct);

        // Step 7: Dispatch to provider-specific poster
        CheckRunPostResult result;
        try
        {
            result = provider == "gitlab"
                ? await gitlabPoster.PostAsync(check.Id, ct)
                : await githubPoster.PostAsync(check.Id, ct);
        }
        catch (Exception)
        {
            // Unexpected exception — the row is left in pending state
            result = new CheckRunPostResult(PrCheckPostStatus.Queued, null,
                "Unexpected error posting check run.",
                provider == "gitlab" ? PrCheckErrorCode.GitLabUnreachable : PrCheckErrorCode.GithubUnavailable);
        }

        // Step 8: Map result to HTTP response
        return result.Status switch
        {
            PrCheckPostStatus.Posted or PrCheckPostStatus.AlreadyPosted =>
                HttpResults.Ok(new PrCheckUploadResponse(
                    "posted",
                    result.CheckRunId,
                    PrCheckId.Format(check.Id))),

            PrCheckPostStatus.Queued when result.Code == PrCheckErrorCode.GithubRateLimited =>
                PrChecksProblem.RateLimited(http),

            PrCheckPostStatus.Queued when result.Code == PrCheckErrorCode.GitLabRateLimited =>
                PrChecksProblem.GitLabRateLimited(http),

            PrCheckPostStatus.Queued =>
                HttpResults.Json(
                    new PrCheckUploadResponse("queued", null, PrCheckId.Format(check.Id)),
                    statusCode: StatusCodes.Status202Accepted),

            PrCheckPostStatus.Failed => result.Code switch
            {
                PrCheckErrorCode.NoInstallation => PrChecksProblem.NoInstallation(http),
                PrCheckErrorCode.RepoNotCovered => PrChecksProblem.RepoNotCovered(http, check.Repo),
                PrCheckErrorCode.InstallationSuspended => PrChecksProblem.InstallationSuspended(http),
                PrCheckErrorCode.InstallationDeleted => PrChecksProblem.InstallationDeleted(http),
                PrCheckErrorCode.TokenLeakDetected => PrChecksProblem.TokenLeakDetected(http),
                PrCheckErrorCode.GitLabTokenRevoked => PrChecksProblem.GitLabTokenRevoked(http),
                PrCheckErrorCode.GitLabNoInstallation => PrChecksProblem.GitLabNoInstallation(http),
                PrCheckErrorCode.GitLabHttpInsecure =>
                    PrChecksProblem.GitLabHttpInsecure(http,
                        result.Error?.Split('\'') is { Length: > 1 } parts ? parts[1] : "unknown"),
                PrCheckErrorCode.GitLabUnreachable => PrChecksProblem.GitLabUnreachable(http),
                _ => PrChecksProblem.PermanentFailure(http),
            },

            _ => PrChecksProblem.Unavailable(http),
        };
    }

    private static async Task<byte[]> ReadBodyAsync(HttpRequest request, CancellationToken ct)
    {
        request.Body.Position = 0;
        using var ms = new MemoryStream();
        await request.Body.CopyToAsync(ms, ct);
        request.Body.Position = 0;
        return ms.ToArray();
    }
}
