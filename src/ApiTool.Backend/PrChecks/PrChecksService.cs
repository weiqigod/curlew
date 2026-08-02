using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.PrChecks;

/// <summary>Business logic for posting and listing PR checks.</summary>
public sealed class PrChecksService(AppDbContext db, TimeProvider clock)
{
    private static readonly HashSet<string> ValidStates = new(StringComparer.OrdinalIgnoreCase)
    {
        "success", "failure",
    };

    /// <summary>
    /// Posts a new PR check for the given org. Validates RBAC membership and
    /// payload schema before persisting.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="request">Request payload.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(PrCheckDto? dto, PrCheckError error, string? message)>
        PostAsync(Guid userId, Guid orgId, PrCheckRequest? request, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return (null, PrCheckError.PermissionDenied, "Permission denied.");

        var (valid, validationMessage) = ValidateRequest(request);
        if (!valid)
            return (null, PrCheckError.InvalidState, validationMessage);

        // Resolve the result id if provided.
        Guid? resultGuid = null;
        if (!string.IsNullOrEmpty(request!.ResultId))
        {
            if (!ResultId.TryParse(request.ResultId, out var parsed))
                return (null, PrCheckError.ResultNotFound, $"result_id '{request.ResultId}' is not a valid result id.");
            resultGuid = parsed;
        }

        var now = clock.GetUtcNow().UtcDateTime;
        var check = new PrCheck
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Repo = request.Repo!,
            Pr = request.Pr!.Value,
            State = request.State!.ToLowerInvariant(),
            ResultId = resultGuid,
            CreatedAt = now,
            ExternalId = Guid.NewGuid(),
        };

        db.PrChecks.Add(check);
        await db.SaveChangesAsync(ct);

        return (ToDto(check), PrCheckError.None, null);
    }

    /// <summary>
    /// Returns a newest-first list of PR checks for the given org.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="limit">Maximum number of results to return (clamped to 1–100).</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<PrCheckDto> checks, PrCheckError error)>
        ListAsync(Guid userId, Guid orgId, int limit, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], PrCheckError.PermissionDenied);

        var clampedLimit = Math.Clamp(limit, 1, 100);

        var rows = await db.PrChecks
            .Where(p => p.OrgId == orgId)
            .OrderByDescending(p => p.CreatedAt)
            .Take(clampedLimit)
            .ToListAsync(ct);

        return (rows.Select(ToDto).ToList(), PrCheckError.None);
    }

    // ── private helpers ──────────────────────────────────────────────────────

    private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
        await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

    private static (bool valid, string? message) ValidateRequest(PrCheckRequest? request)
    {
        if (request is null)
            return (false, "Request body is required.");

        if (string.IsNullOrWhiteSpace(request.Repo))
            return (false, "repo is required.");

        if (request.Pr is null || request.Pr.Value <= 0)
            return (false, "pr must be a positive integer.");

        if (string.IsNullOrWhiteSpace(request.State) || !ValidStates.Contains(request.State))
            return (false, $"state must be 'success' or 'failure'.");

        return (true, null);
    }

    private static PrCheckDto ToDto(PrCheck p) =>
        new(
            Id: PrCheckId.Format(p.Id),
            Repo: p.Repo,
            Pr: p.Pr,
            State: p.State,
            ResultId: p.ResultId.HasValue ? ResultId.Format(p.ResultId.Value) : null,
            CreatedAt: p.CreatedAt,
            PostedAt: p.PostedAt,
            CheckRunId: p.CheckRunId);
}
