// Spec refs: docs/SPECIFICATION.md:6821 (INSERT … ON CONFLICT DO NOTHING RETURNING),
// :6843 (5-failure quarantine budget), :6852 (90-day cleanup retention).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Webhooks;

/// <summary>Encapsulates all DB writes against <c>stripe_webhook_events</c>.</summary>
public sealed class StripeWebhookStore(AppDbContext db, TimeProvider clock)
{
    /// <summary>Quarantine threshold per spec :6843.</summary>
    public const int QuarantineThreshold = 5;

    /// <summary>
    /// Atomically inserts a new pending row keyed by <paramref name="eventId"/>. Returns
    /// <see cref="IdempotentInsertResult.Inserted"/> = true when this was the first time
    /// we saw the event, false when a row already existed (duplicate delivery).
    /// </summary>
    public async Task<IdempotentInsertResult> TryInsertPendingAsync(
        string eventId, string eventType, string payloadJson, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        if (db.Database.IsNpgsql())
        {
            // Postgres path — single statement, atomic per spec :6821.
            // Returns 1 row if inserted, 0 rows if duplicate.
            var inserted = await db.StripeWebhookEvents
                .FromSqlInterpolated($@"
                    INSERT INTO stripe_webhook_events
                        (event_id, event_type, received_at, status, payload, attempt_count)
                    VALUES
                        ({eventId}, {eventType}, {now}, 'pending', {payloadJson}::jsonb, 0)
                    ON CONFLICT (event_id) DO NOTHING
                    RETURNING *")
                .AsNoTracking()
                .ToListAsync(ct);

            if (inserted.Count > 0)
                return new IdempotentInsertResult(true, null, null);
            var (existingAt, existingStatus) = await ExistingRowMetaAsync(eventId, ct);
            return new IdempotentInsertResult(false, existingAt, existingStatus);
        }

        // SQLite / InMemory path — LINQ guarded by a TryAdd then catch unique-violation.
        var existingRow = await db.StripeWebhookEvents
            .Where(x => x.EventId == eventId)
            .FirstOrDefaultAsync(ct);
        if (existingRow is not null)
            return new IdempotentInsertResult(false, existingRow.ReceivedAt, existingRow.Status);

        db.StripeWebhookEvents.Add(new StripeWebhookEvent
        {
            EventId = eventId,
            EventType = eventType,
            ReceivedAt = now,
            Status = "pending",
            PayloadJson = payloadJson,
            AttemptCount = 0,
        });
        try
        {
            await db.SaveChangesAsync(ct);
            return new IdempotentInsertResult(true, null);
        }
        catch (DbUpdateException) // race: another writer inserted between our SELECT and INSERT.
        {
            // Detach the failed entity so subsequent SaveChanges work.
            db.ChangeTracker.Clear();
            var racedRow = await db.StripeWebhookEvents.FirstOrDefaultAsync(x => x.EventId == eventId, ct);
            return new IdempotentInsertResult(false, racedRow?.ReceivedAt, racedRow?.Status ?? "pending");
        }
    }

    /// <summary>Marks the row as processed.</summary>
    public async Task MarkProcessedAsync(string eventId, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        if (db.Database.IsNpgsql() || db.Database.IsSqlite())
        {
            await db.StripeWebhookEvents
                .Where(x => x.EventId == eventId)
                .ExecuteUpdateAsync(s => s
                    .SetProperty(x => x.Status, "processed")
                    .SetProperty(x => x.ProcessedAt, (DateTime?)now), ct);
        }
        else
        {
            // EF InMemory provider (used in BackendFactory integration tests) doesn't support ExecuteUpdateAsync.
            var row = await db.StripeWebhookEvents.FirstOrDefaultAsync(x => x.EventId == eventId, ct);
            if (row is not null)
            {
                row.Status = "processed";
                row.ProcessedAt = now;
                await db.SaveChangesAsync(ct);
            }
        }
    }

    /// <summary>
    /// Records a handler failure. Returns the new attempt_count after increment;
    /// when it reaches <see cref="QuarantineThreshold"/>, the status is updated to
    /// <c>quarantined</c> in the same call.
    /// </summary>
    public async Task<int> RecordFailureAsync(string eventId, string errorMessage, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        var row = await db.StripeWebhookEvents.FirstOrDefaultAsync(x => x.EventId == eventId, ct)
            ?? throw new InvalidOperationException($"stripe_webhook_events row missing for {eventId}");
        row.AttemptCount += 1;
        row.LastError = errorMessage;
        row.LastErrorAt = now;
        if (row.AttemptCount >= QuarantineThreshold)
            row.Status = "quarantined";
        await db.SaveChangesAsync(ct);
        return row.AttemptCount;
    }

    /// <summary>Deletes processed rows older than <paramref name="cutoff"/>. Quarantined rows are retained.</summary>
    public async Task<int> DeleteOlderThanAsync(DateTime cutoff, CancellationToken ct)
    {
        if (db.Database.IsNpgsql() || db.Database.IsSqlite())
        {
            return await db.StripeWebhookEvents
                .Where(x => x.Status == "processed" && x.ReceivedAt < cutoff)
                .ExecuteDeleteAsync(ct);
        }

        // EF InMemory fallback.
        var rows = await db.StripeWebhookEvents
            .Where(x => x.Status == "processed" && x.ReceivedAt < cutoff)
            .ToListAsync(ct);
        db.StripeWebhookEvents.RemoveRange(rows);
        await db.SaveChangesAsync(ct);
        return rows.Count;
    }

    private async Task<(DateTime? ReceivedAt, string Status)> ExistingRowMetaAsync(string eventId, CancellationToken ct)
    {
        var row = await db.StripeWebhookEvents
            .Where(x => x.EventId == eventId)
            .Select(x => new { x.ReceivedAt, x.Status })
            .FirstOrDefaultAsync(ct);
        return row is null ? (null, "pending") : (row.ReceivedAt, row.Status);
    }
}

/// <summary>Result of <see cref="StripeWebhookStore.TryInsertPendingAsync"/>.</summary>
/// <param name="Inserted">True if a new row was inserted; false if duplicate.</param>
/// <param name="ExistingReceivedAt">When duplicate, the original row's received_at (else null).</param>
/// <param name="ExistingStatus">
/// When duplicate, the status of the existing row — one of <c>pending</c>, <c>processed</c>,
/// <c>quarantined</c>. Null when <see cref="Inserted"/> is true.
/// </param>
public sealed record IdempotentInsertResult(bool Inserted, DateTime? ExistingReceivedAt, string? ExistingStatus = null);
