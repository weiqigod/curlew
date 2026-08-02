using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Notifications;

/// <summary>
/// Business logic for creating notification rules, listing deliveries, and
/// finding matching rules for a fired event.
/// </summary>
public sealed class NotificationsService(AppDbContext db, TimeProvider clock)
{
    /// <summary>
    /// Creates a new notification rule for the given organization.
    /// Requires Owner or Admin role.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="req">Rule creation payload.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple of (dto, error, message). On success, error is <see cref="NotificationError.None"/>.
    /// </returns>
    public async Task<(NotificationRuleDto? dto, NotificationError error, string? message)>
        CreateRuleAsync(Guid userId, Guid orgId, CreateNotificationRuleRequest? req, CancellationToken ct)
    {
        if (!await IsAdminAsync(userId, orgId, ct))
            return (null, NotificationError.PermissionDenied, "Permission denied.");

        if (req is null)
            return (null, NotificationError.InvalidChannel, "Request body is required.");

        // Validate channel
        if (string.IsNullOrWhiteSpace(req.Channel) ||
            !Enum.TryParse<NotificationChannel>(req.Channel, ignoreCase: true, out var channel))
        {
            return (null, NotificationError.InvalidChannel, $"Unknown channel: '{req.Channel}'. Valid values: slack, email.");
        }

        // Validate target
        if (string.IsNullOrWhiteSpace(req.Target))
            return (null, NotificationError.InvalidTarget, "target is required.");

        if (channel == NotificationChannel.Slack && !req.Target.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            return (null, NotificationError.InvalidTarget, "Slack target must be an HTTPS URL.");

        if (channel == NotificationChannel.Email && !req.Target.Contains('@'))
            return (null, NotificationError.InvalidTarget, "Email target must be a valid email address.");

        // Validate events
        if (req.On is null || req.On.Count == 0)
            return (null, NotificationError.InvalidEvents, "on must contain at least one event.");

        var events = new List<NotificationEvent>(req.On.Count);
        foreach (var raw in req.On)
        {
            if (!NotificationEventHelper.TryParse(raw, out var evt))
                return (null, NotificationError.InvalidEvents, $"Unknown event: '{raw}'. Valid values: run_failed, flaky.");
            events.Add(evt);
        }

        var now = clock.GetUtcNow().UtcDateTime;
        var rule = new NotificationRule
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Channel = channel,
            Target = req.Target,
            OnEvents = NotificationEventHelper.FormatStoredEvents(events),
            CreatedBy = userId,
            CreatedAt = now,
        };

        db.NotificationRules.Add(rule);
        await db.SaveChangesAsync(ct);

        return (ToRuleDto(rule), NotificationError.None, null);
    }

    /// <summary>
    /// Lists notification rules for the given organization. Requires any org membership.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<NotificationRuleDto> rules, NotificationError error)>
        ListRulesAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], NotificationError.PermissionDenied);

        var rows = await db.NotificationRules
            .Where(r => r.OrgId == orgId)
            .OrderBy(r => r.CreatedAt)
            .ToListAsync(ct);

        return (rows.Select(ToRuleDto).ToList(), NotificationError.None);
    }

    /// <summary>
    /// Lists delivery attempts for the given organization, newest first.
    /// Requires any org membership.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="limit">Maximum number of deliveries to return (clamped to 1–100).</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(IReadOnlyList<NotificationDeliveryDto> deliveries, NotificationError error)>
        ListDeliveriesAsync(Guid userId, Guid orgId, int limit, CancellationToken ct)
    {
        if (!await IsMemberAsync(userId, orgId, ct))
            return ([], NotificationError.PermissionDenied);

        var clampedLimit = Math.Clamp(limit, 1, 100);

        var rows = await db.NotificationDeliveries
            .Where(d => d.OrgId == orgId)
            .OrderByDescending(d => d.AttemptedAt)
            .Take(clampedLimit)
            .ToListAsync(ct);

        return (rows.Select(ToDeliveryDto).ToList(), NotificationError.None);
    }

    /// <summary>
    /// Deletes a notification rule for the given organization.
    /// Requires Owner or Admin role.
    /// </summary>
    /// <param name="userId">The requesting user's id.</param>
    /// <param name="orgId">Target organization id.</param>
    /// <param name="ruleId">The rule to delete.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns><see cref="NotificationError.None"/> on success; otherwise the relevant error code.</returns>
    public async Task<NotificationError> DeleteRuleAsync(
        Guid userId, Guid orgId, Guid ruleId, CancellationToken ct)
    {
        if (!await IsAdminAsync(userId, orgId, ct))
            return NotificationError.PermissionDenied;

        var rule = await db.NotificationRules
            .SingleOrDefaultAsync(r => r.Id == ruleId && r.OrgId == orgId, ct);
        if (rule is null)
            return NotificationError.NotFound;

        db.NotificationRules.Remove(rule);
        await db.SaveChangesAsync(ct);
        return NotificationError.None;
    }

    /// <summary>
    /// Returns the notification rules that match a given event for an organization.
    /// Used by the dispatcher to determine which rules fire.
    /// </summary>
    /// <param name="orgId">The organization id.</param>
    /// <param name="evt">The event that fired.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<IReadOnlyList<NotificationRule>> FindMatchingRulesAsync(
        Guid orgId, NotificationEvent evt, CancellationToken ct)
    {
        var evtWire = NotificationEventHelper.Format(evt);

        // Load all org rules, then filter in-process to perform exact token matching on the
        // pipe-separated OnEvents column (SQLite has no array type; LIKE would risk partial matches).
        var allRules = await db.NotificationRules
            .Where(r => r.OrgId == orgId)
            .ToListAsync(ct);

        return allRules
            .Where(r => ContainsEvent(r.OnEvents, evtWire))
            .ToList();
    }

    /// <summary>
    /// Persists a delivery record to the database.
    /// </summary>
    /// <param name="delivery">The delivery entity to save.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task RecordDeliveryAsync(NotificationDelivery delivery, CancellationToken ct)
    {
        db.NotificationDeliveries.Add(delivery);
        await db.SaveChangesAsync(ct);
    }

    // ── private helpers ──────────────────────────────────────────────────────

    private async Task<bool> IsMemberAsync(Guid userId, Guid orgId, CancellationToken ct) =>
        await db.OrganizationMembers.AnyAsync(m => m.OrgId == orgId && m.UserId == userId, ct);

    private async Task<bool> IsAdminAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        var member = await db.OrganizationMembers
            .SingleOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        return member is not null && member.Role is OrgRole.Owner or OrgRole.Admin;
    }

    private static bool ContainsEvent(string stored, string eventWire)
    {
        // Split on '|' and check for an exact token match to avoid substring false-positives.
        var parts = stored.Split('|');
        return parts.Any(p => string.Equals(p, eventWire, StringComparison.Ordinal));
    }

    private static NotificationRuleDto ToRuleDto(NotificationRule r)
    {
        var events = NotificationEventHelper.ParseStoredEvents(r.OnEvents);
        var onWire = events?.Select(NotificationEventHelper.Format).ToList()
            ?? [r.OnEvents];

        return new NotificationRuleDto(
            Id: NotificationRuleId.Format(r.Id),
            Channel: r.Channel.ToString().ToLowerInvariant(),
            Target: r.Target,
            On: onWire,
            CreatedAt: r.CreatedAt);
    }

    private static NotificationDeliveryDto ToDeliveryDto(NotificationDelivery d) =>
        new(
            Id: NotificationDeliveryId.Format(d.Id),
            RuleId: NotificationRuleId.Format(d.RuleId),
            Channel: d.Channel.ToString().ToLowerInvariant(),
            Status: d.Status.ToString().ToLowerInvariant(),
            ResponseCode: d.ResponseCode,
            AttemptCount: d.AttemptCount,
            ErrorMessage: d.ErrorMessage,
            AttemptedAt: d.AttemptedAt);
}
