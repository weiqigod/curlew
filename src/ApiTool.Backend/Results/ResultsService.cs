using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results.Dashboard;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Results;

/// <summary>Business logic for ingesting, listing, and retrieving test results.</summary>
public sealed class ResultsService(
    AppDbContext db,
    TimeProvider clock,
    IResultIngestedNotifier notifier)
{
    /// <summary>Maximum number of items allowed per upload request.</summary>
    public const int MaxItemsPerRequest = 5000;

    /// <summary>Maximum character length for per-item messages.</summary>
    public const int MaxMessageLength = 4000;

    /// <summary>
    /// Ingests a test run upload for the given org. Validates RBAC membership
    /// and payload schema before persisting. Fires the notifier after a successful save.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="request">Upload payload.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple of (dto, error, message, fieldPointer).
    /// On success, error is <see cref="ResultError.None"/> and dto is populated.
    /// On failure, dto is null and error indicates the reason; fieldPointer points to the
    /// invalid field (e.g. "/pass_count") when error is <see cref="ResultError.InvalidSchema"/>.
    /// </returns>
    public async Task<(ResultDto? dto, ResultError error, string? message, string? fieldPointer)>
        IngestAsync(Guid userId, Guid orgId, UploadResultRequest? request, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return (null, ResultError.PermissionDenied, "Permission denied.", null);

        // Schema validation — returns first failing field as a JSON pointer.
        var (valid, validationMessage, fieldPointer) = ValidateRequest(request);
        if (!valid)
            return (null, ResultError.InvalidSchema, validationMessage, fieldPointer);

        var now = clock.GetUtcNow().UtcDateTime;
        var resultId = Guid.NewGuid();

        var result = new Result
        {
            Id = resultId,
            OrgId = orgId,
            UploadedBy = userId,
            CollectionName = request!.CollectionName!,
            RunAt = request.RunAt!.Value,
            DurationMs = request.DurationMs!.Value,
            PassCount = request.PassCount!.Value,
            FailCount = request.FailCount!.Value,
            SkippedCount = request.SkippedCount ?? 0,
            TriggeredBy = request.TriggeredBy,
            GitSha = request.GitSha,
            CreatedAt = now,
        };

        db.Results.Add(result);

        var items = request.Items ?? [];
        for (var i = 0; i < items.Count; i++)
        {
            var item = items[i];
            var method = item.Method?.Trim().ToUpperInvariant();
            var pathTemplate = PathTemplateExtractor.Extract(method, item.RequestUrl);

            db.ResultItems.Add(new ResultItem
            {
                Id = Guid.NewGuid(),
                ResultId = resultId,
                Ordinal = i,
                Name = item.Name!,
                Status = ParseStatus(item.Status!),
                DurationMs = item.DurationMs!.Value,
                Message = item.Message is { Length: > MaxMessageLength }
                    ? item.Message[..MaxMessageLength]
                    : item.Message,
                Method = method,
                RequestUrl = item.RequestUrl,
                PathTemplate = pathTemplate,
            });
        }

        await db.SaveChangesAsync(ct);
        await notifier.NotifyAsync(orgId, resultId, ct);

        return (ToDto(result), ResultError.None, null, null);
    }

    /// <summary>
    /// Returns a newest-first list of results for the given org.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="limit">Maximum number of results to return (clamped to 1–100, default 10).</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<ResultDto> results, ResultError error)>
        ListAsync(Guid userId, Guid orgId, int limit, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], ResultError.PermissionDenied);

        var clampedLimit = Math.Clamp(limit, 1, 100);

        var rows = await db.Results
            .Where(r => r.OrgId == orgId)
            .OrderByDescending(r => r.CreatedAt)
            .Take(clampedLimit)
            .ToListAsync(ct);

        return (rows.Select(ToDto).ToList(), ResultError.None);
    }

    /// <summary>
    /// Returns the full detail (header + per-test items) for a result by id.
    /// Returns <see cref="ResultError.NotFound"/> if the result does not exist or
    /// the calling user is not a member of the owning org.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="resultId">The result's internal id.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(ResultDetailDto? dto, ResultError error)>
        GetDetailAsync(Guid userId, Guid resultId, CancellationToken ct)
    {
        var result = await db.Results.FindAsync([resultId], ct);
        if (result is null)
            return (null, ResultError.NotFound);

        if (!await IsMemberAsync(userId, result.OrgId, ct))
            return (null, ResultError.NotFound);

        var items = await db.ResultItems
            .Where(i => i.ResultId == resultId)
            .OrderBy(i => i.Ordinal)
            .ToListAsync(ct);

        var dto = new ResultDetailDto(
            Id: ResultId.Format(result.Id),
            CollectionName: result.CollectionName,
            PassCount: result.PassCount,
            FailCount: result.FailCount,
            SkippedCount: result.SkippedCount,
            DurationMs: result.DurationMs,
            RunAt: result.RunAt,
            CreatedAt: result.CreatedAt,
            TriggeredBy: result.TriggeredBy,
            GitSha: result.GitSha,
            Items: items.Select(i => new ResultItemDto(
                Name: i.Name,
                Status: i.Status.ToString().ToLowerInvariant(),
                DurationMs: i.DurationMs,
                Message: i.Message)).ToList());

        return (dto, ResultError.None);
    }

    // ── private helpers ──────────────────────────────────────────────────────

    private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
        await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

    private static (bool valid, string? message, string? pointer) ValidateRequest(UploadResultRequest? request)
    {
        if (request is null)
            return (false, "Request body is required.", "/");

        if (string.IsNullOrWhiteSpace(request.CollectionName))
            return (false, "collection_name is required.", "/collection_name");

        if (request.RunAt is null)
            return (false, "run_at is required.", "/run_at");

        if (request.DurationMs is null || request.DurationMs.Value < 0)
            return (false, "duration_ms must be a non-negative integer.", "/duration_ms");

        if (request.PassCount is null || request.PassCount.Value < 0)
            return (false, "pass_count must be a non-negative integer.", "/pass_count");

        if (request.FailCount is null || request.FailCount.Value < 0)
            return (false, "fail_count must be a non-negative integer.", "/fail_count");

        if (request.Items is null)
            return (false, "items is required.", "/items");

        if (request.Items.Count > MaxItemsPerRequest)
            return (false, $"items must not exceed {MaxItemsPerRequest} entries.", "/items");

        for (var i = 0; i < request.Items.Count; i++)
        {
            var item = request.Items[i];

            if (string.IsNullOrWhiteSpace(item.Name))
                return (false, $"items[{i}].name is required.", $"/items/{i}/name");

            if (string.IsNullOrWhiteSpace(item.Status) || !TryParseStatus(item.Status, out _))
                return (false, $"items[{i}].status is invalid.", $"/items/{i}/status");

            if (item.DurationMs is null || item.DurationMs.Value < 0)
                return (false, $"items[{i}].duration_ms must be a non-negative integer.", $"/items/{i}/duration_ms");
        }

        return (true, null, null);
    }

    private static ResultStatus ParseStatus(string status) =>
        Enum.TryParse<ResultStatus>(status, ignoreCase: true, out var s) ? s : ResultStatus.Error;

    private static bool TryParseStatus(string status, out ResultStatus result) =>
        Enum.TryParse(status, ignoreCase: true, out result);

    private static ResultDto ToDto(Result r) =>
        new(
            Id: ResultId.Format(r.Id),
            CollectionName: r.CollectionName,
            PassCount: r.PassCount,
            FailCount: r.FailCount,
            SkippedCount: r.SkippedCount,
            DurationMs: r.DurationMs,
            RunAt: r.RunAt,
            CreatedAt: r.CreatedAt,
            TriggeredBy: r.TriggeredBy,
            GitSha: r.GitSha);
}
