using ApiTool.Backend.Audit;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Maps the GDPR account-deletion state-machine endpoints (M18-005, v4-5):
/// <list type="bullet">
///   <item><c>POST /api/v1/users/me/deletion-requests</c> — initiate 30-day deletion (re-auth required)</item>
///   <item><c>POST /api/v1/users/me/deletion-requests/cancel</c> — abort within the 30-day window</item>
///   <item><c>GET  /api/v1/users/me/deletion-requests/status</c> — report current state</item>
/// </list>
/// </summary>
public static class UserDeletionEndpoints
{
    private static readonly TimeSpan CooldownDuration = TimeSpan.FromDays(30);

    /// <summary>Registers all three deletion state-machine endpoints.</summary>
    public static IEndpointRouteBuilder MapUserDeletionEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/users/me/deletion-requests", RequestDeletion)
            .RequireAuthorization()
            .DisableAntiforgery()
            .Produces<UserDeletionRequestDto>(StatusCodes.Status202Accepted)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces<OwnerCannotLeaveProblemDetails>(StatusCodes.Status409Conflict)
            .WithName("RequestUserDeletion")
            .WithTags("UserDataDeletion")
            .WithSummary("Initiate a GDPR account deletion (30-day cooldown). Requires X-Reauth-Token. Returns 409 owner_cannot_leave with blocking_orgs[] when user is sole owner of org(s) with other members (v4-7).");

        app.MapPost("/api/v1/users/me/deletion-requests/cancel", CancelDeletion)
            .RequireAuthorization()
            .DisableAntiforgery()
            .Produces(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status404NotFound)
            .WithName("CancelUserDeletion")
            .WithTags("UserDataDeletion")
            .WithSummary("Cancel a pending GDPR account deletion request.");

        app.MapGet("/api/v1/users/me/deletion-requests/status", GetStatus)
            .RequireAuthorization()
            .Produces<UserDeletionStatusDto>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status404NotFound)
            .WithName("GetUserDeletionStatus")
            .WithTags("UserDataDeletion")
            .WithSummary("Get the current GDPR deletion request status.");

        return app;
    }

    private static async Task<IResult> RequestDeletion(
        HttpContext httpContext,
        CurrentUserAccessor users,
        IDeletionReauthService reauthService,
        LastAdminProtectionService lastAdmin,
        AppDbContext db,
        IEmailQueue emailQueue,
        IOptions<AppOptions> appOptions,
        TimeProvider clock,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        // Require X-Reauth-Token header
        var rawToken = httpContext.Request.Headers["X-Reauth-Token"].FirstOrDefault();
        if (rawToken is null)
            return DeletionProblems.ReauthRequired();

        // AlreadyPending check fires BEFORE last-admin protection so that a user who is
        // both already-pending AND a blocking owner receives already_pending (not
        // owner_cannot_leave). This preserves the documented check order from the plan.
        var user = await db.Users.FindAsync([userId.Value], ct);
        if (user is null)
            return HttpResults.Unauthorized();
        if (user.PendingDeletionAt is not null)
            return DeletionProblems.AlreadyPending();

        // Last-admin protection check — BEFORE consuming the re-auth token so that
        // a blocked request can be retried with the same token after ownership transfer.
        var (blocking, cascadeOrgIds) = await lastAdmin.ClassifyOwnedOrgsAsync(userId.Value, ct);
        if (blocking.Count > 0)
            return DeletionProblems.OwnerCannotLeave(blocking);

        // Consume the re-auth token (validates freshness + single-use)
        var consumeResult = await reauthService.ConsumeAsync(userId.Value, rawToken, ct);
        return consumeResult switch
        {
            ReauthError.None => await DoRequestDeletionAsync(
                user, cascadeOrgIds, db, emailQueue, appOptions, clock, ct),
            ReauthError.TokenExpired  => DeletionProblems.ReauthExpired(),
            ReauthError.TokenConsumed => DeletionProblems.ReauthConsumed(),
            _                         => DeletionProblems.ReauthInvalid(),
        };
    }

    private static async Task<IResult> DoRequestDeletionAsync(
        User user,
        IReadOnlyList<Guid> cascadeOrgIds,
        AppDbContext db,
        IEmailQueue emailQueue,
        IOptions<AppOptions> appOptions,
        TimeProvider clock,
        CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        user.PendingDeletionAt = now;

        // Mark cascade orgs (sole-owner, zero-other-members) as PendingDeletion atomically.
        if (cascadeOrgIds.Count > 0)
        {
            var cascadeOrgs = await db.Organizations
                .Where(o => cascadeOrgIds.Contains(o.Id))
                .ToListAsync(ct);
            foreach (var org in cascadeOrgs)
                org.Status = OrgStatus.PendingDeletion;
        }

        await db.SaveChangesAsync(ct);

        var finalizesAt = now + CooldownDuration;
        var cancelUrl = $"{appOptions.Value.WebAppUrl.TrimEnd('/')}/account/data/cancel-deletion";

        await emailQueue.EnqueueAsync(new EmailMessage(
            To: user.Email,
            TemplateSlug: "account_deletion_initiated",
            Variables: new Dictionary<string, string>
            {
                ["user_email"]         = user.Email,
                ["cancel_url"]         = cancelUrl,
                ["finalizes_at_local"] = finalizesAt.ToString("yyyy-MM-dd HH:mm 'UTC'"),
            },
            EnqueuedAt: clock.GetUtcNow()), ct);

        return HttpResults.Accepted(
            uri: null,
            value: new UserDeletionRequestDto(
                FinalizesAt:      finalizesAt.ToString("O"),
                CancellableUntil: finalizesAt.ToString("O"),
                CancelUrl:        cancelUrl));
    }

    private static async Task<IResult> CancelDeletion(
        CurrentUserAccessor users,
        AppDbContext db,
        IAuditWriter audit,
        TimeProvider clock,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        var user = await db.Users.FindAsync([userId], ct);
        if (user is null || user.PendingDeletionAt is null)
            return DeletionProblems.NoPendingRequest();

        user.PendingDeletionAt = null;

        // Reverse any cascade orgs that were atomically marked PendingDeletion when
        // the deletion was initiated. Cascade orgs are those owned by this user that have
        // zero other members (i.e., only the owner row). We restore them to Active and
        // emit an org.deletion_canceled audit event per reversed org.
        //
        // Heuristic assumption (pre-launch): the query `OwnerId == userId && Status == PendingDeletion`
        // is used as a proxy for "orgs that were cascade-marked by this user's deletion request".
        // This is safe as long as no other code path independently sets an org to PendingDeletion
        // for a user-owned org without the user's deletion being the cause. If an independent
        // PendingDeletion path is added in a future milestone, this should be replaced with
        // an explicit tracking column (e.g., `cascade_deletion_at`) or a join table that records
        // which deletion request triggered each cascade.
        // TODO(M19+): store cascade-marked org IDs on the deletion-request row to avoid the heuristic.
        var cascadeOrgs = await db.Organizations
            .Where(o => o.OwnerId == userId.Value && o.Status == OrgStatus.PendingDeletion)
            .ToListAsync(ct);

        foreach (var org in cascadeOrgs)
        {
            org.Status = OrgStatus.Active;
            audit.Append(new AuditEvent(
                OrgId:      org.Id,
                ActorId:    userId.Value,
                EventType:  "org.deletion_canceled",
                TargetType: "org",
                TargetId:   org.Id));
        }

        audit.Append(new AuditEvent(
            OrgId:      Guid.Empty,
            ActorId:    userId.Value,
            EventType:  "account.deletion_cancelled",
            TargetType: "user",
            TargetId:   userId.Value));

        await db.SaveChangesAsync(ct);

        return HttpResults.Ok(new { cancelled = true });
    }

    private static async Task<IResult> GetStatus(
        CurrentUserAccessor users,
        AppDbContext db,
        TimeProvider clock,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        var user = await db.Users
            .Where(u => u.Id == userId.Value)
            .Select(u => new { u.PendingDeletionAt, u.AnonymisedAt })
            .FirstOrDefaultAsync(ct);

        if (user is null || (user.PendingDeletionAt is null && user.AnonymisedAt is null))
            return HttpResults.NotFound();

        string? finalizesAt = user.PendingDeletionAt.HasValue
            ? (user.PendingDeletionAt.Value + CooldownDuration).ToString("O")
            : null;

        return HttpResults.Ok(new UserDeletionStatusDto(
            PendingDeletionAt: user.PendingDeletionAt?.ToString("O"),
            FinalizesAt:       finalizesAt,
            AnonymisedAt:      user.AnonymisedAt?.ToString("O")));
    }
}
