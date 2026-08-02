// Refs docs/SPECIFICATION.md:8586-8595 (INSERT … ON CONFLICT DO NOTHING),
// :8597 (5-failure quarantine budget), :8595 (90-day retention).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>Encapsulates all DB writes against <c>github_webhook_events</c>.</summary>
public sealed class GithubWebhookStore(AppDbContext db, TimeProvider clock)
{
    /// <summary>Quarantine threshold — same as Stripe (spec :8597).</summary>
    public const int QuarantineThreshold = 5;

    /// <summary>
    /// Atomically inserts a new pending row keyed by <paramref name="deliveryId"/>.
    /// Returns Inserted=true on first delivery, false on duplicate.
    /// </summary>
    public async Task<GithubIdempotentInsertResult> TryInsertPendingAsync(
        Guid deliveryId, string eventType, string? action, string payloadJson, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        if (db.Database.IsNpgsql())
        {
            // Postgres path — single atomic statement. Returns row if inserted, nothing if duplicate.
            var inserted = await db.GithubWebhookEvents
                .FromSqlInterpolated($@"
                    INSERT INTO github_webhook_events
                        (delivery_id, event_type, action, received_at, status, payload, attempt_count)
                    VALUES
                        ({deliveryId}, {eventType}, {action}, {now}, 'pending', {payloadJson}::jsonb, 0)
                    ON CONFLICT (delivery_id) DO NOTHING
                    RETURNING *")
                .AsNoTracking()
                .ToListAsync(ct);

            if (inserted.Count > 0)
                return new GithubIdempotentInsertResult(true, null, null);
            var (existingAt, existingStatus) = await ExistingRowMetaAsync(deliveryId, ct);
            return new GithubIdempotentInsertResult(false, existingAt, existingStatus);
        }

        // SQLite / InMemory path — LINQ guarded by check then insert.
        var existingRow = await db.GithubWebhookEvents
            .Where(x => x.DeliveryId == deliveryId)
            .FirstOrDefaultAsync(ct);
        if (existingRow is not null)
            return new GithubIdempotentInsertResult(false, existingRow.ReceivedAt, existingRow.Status);

        db.GithubWebhookEvents.Add(new GithubWebhookEvent
        {
            DeliveryId = deliveryId,
            EventType = eventType,
            Action = action,
            ReceivedAt = now,
            Status = "pending",
            PayloadJson = payloadJson,
            AttemptCount = 0,
        });
        try
        {
            await db.SaveChangesAsync(ct);
            return new GithubIdempotentInsertResult(true, null, null);
        }
        catch (DbUpdateException) // race: another writer inserted between our SELECT and INSERT.
        {
            db.ChangeTracker.Clear();
            var racedRow = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId, ct);
            return new GithubIdempotentInsertResult(false, racedRow?.ReceivedAt, racedRow?.Status ?? "pending");
        }
    }

    /// <summary>Marks the row as processed.</summary>
    public async Task MarkProcessedAsync(Guid deliveryId, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        if (db.Database.IsNpgsql() || db.Database.IsSqlite())
        {
            await db.GithubWebhookEvents
                .Where(x => x.DeliveryId == deliveryId)
                .ExecuteUpdateAsync(s => s
                    .SetProperty(x => x.Status, "processed")
                    .SetProperty(x => x.ProcessedAt, (DateTime?)now), ct);
        }
        else
        {
            // EF InMemory provider (used in BackendFactory integration tests) doesn't support ExecuteUpdateAsync.
            var row = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId, ct);
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
    public async Task<int> RecordFailureAsync(Guid deliveryId, string errorMessage, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        var row = await db.GithubWebhookEvents.FirstOrDefaultAsync(x => x.DeliveryId == deliveryId, ct)
            ?? throw new InvalidOperationException($"github_webhook_events row missing for {deliveryId}");
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
            return await db.GithubWebhookEvents
                .Where(x => x.Status == "processed" && x.ReceivedAt < cutoff)
                .ExecuteDeleteAsync(ct);
        }

        // EF InMemory fallback.
        var rows = await db.GithubWebhookEvents
            .Where(x => x.Status == "processed" && x.ReceivedAt < cutoff)
            .ToListAsync(ct);
        db.GithubWebhookEvents.RemoveRange(rows);
        await db.SaveChangesAsync(ct);
        return rows.Count;
    }

    private async Task<(DateTime? ReceivedAt, string Status)> ExistingRowMetaAsync(
        Guid deliveryId, CancellationToken ct)
    {
        var row = await db.GithubWebhookEvents
            .Where(x => x.DeliveryId == deliveryId)
            .Select(x => new { x.ReceivedAt, x.Status })
            .FirstOrDefaultAsync(ct);
        return row is null ? (null, "pending") : (row.ReceivedAt, row.Status);
    }
}

/// <summary>Result of <see cref="GithubWebhookStore.TryInsertPendingAsync"/>.</summary>
/// <param name="Inserted">True if a new row was inserted; false if duplicate.</param>
/// <param name="ExistingReceivedAt">When duplicate, the original row's received_at (else null).</param>
/// <param name="ExistingStatus">When duplicate, the status of the existing row. Null when Inserted is true.</param>
public sealed record GithubIdempotentInsertResult(
    bool Inserted, DateTime? ExistingReceivedAt, string? ExistingStatus = null);
