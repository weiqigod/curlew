// Refs docs/SPECIFICATION.md:10922-10943 (schema), :9261 (idempotency contract).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Encapsulates all DB writes against <c>gitlab_webhook_events</c>.
/// Refs docs/SPECIFICATION.md:10922-10943 (schema), :9261 (idempotency contract).
/// </summary>
public sealed class GitLabWebhookStore(AppDbContext db, TimeProvider clock)
{
    /// <summary>Quarantine threshold — 5 consecutive dispatcher failures quarantine the event row.</summary>
    public const int QuarantineThreshold = 5;

    /// <summary>
    /// Atomically inserts a pending row keyed by <paramref name="eventUuid"/>.
    /// Returns Inserted=true on first delivery, false on duplicate.
    /// </summary>
    public async Task<GitLabIdempotentInsertResult> TryInsertPendingAsync(
        string eventUuid, string eventType, Guid installationId,
        string payloadJson, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        // SQLite / InMemory path — LINQ guarded by check-then-insert.
        // (No INSERT … ON CONFLICT path for SQLite; Postgres path not implemented
        // in this slice since the backend runs on SQLite for development.)
        var existingRow = await db.GitLabWebhookEvents
            .Where(x => x.EventUuid == eventUuid)
            .FirstOrDefaultAsync(ct);

        if (existingRow is not null)
        {
            return new GitLabIdempotentInsertResult(
                Inserted: false,
                ExistingReceivedAt: existingRow.ReceivedAt,
                ExistingQuarantined: existingRow.QuarantinedAt.HasValue,
                ExistingProcessed: existingRow.ProcessedAt.HasValue);
        }

        db.GitLabWebhookEvents.Add(new GitLabWebhookEvent
        {
            Id = Guid.NewGuid(),
            EventUuid = eventUuid,
            EventType = eventType,
            InstallationId = installationId,
            ReceivedAt = now,
            FailureCount = 0,
            PayloadJson = payloadJson,
        });

        try
        {
            await db.SaveChangesAsync(ct);
            return new GitLabIdempotentInsertResult(Inserted: true, ExistingReceivedAt: null, ExistingQuarantined: false);
        }
        catch (DbUpdateException)
        {
            // Race: another writer inserted between our SELECT and INSERT (unique index violation).
            db.ChangeTracker.Clear();
            var racedRow = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid, ct);
            return new GitLabIdempotentInsertResult(
                Inserted: false,
                ExistingReceivedAt: racedRow?.ReceivedAt,
                ExistingQuarantined: racedRow?.QuarantinedAt.HasValue ?? false,
                ExistingProcessed: racedRow?.ProcessedAt.HasValue ?? false);
        }
    }

    /// <summary>Marks the row as processed by setting <c>processed_at</c>.</summary>
    public async Task MarkProcessedAsync(string eventUuid, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;

        if (db.Database.IsNpgsql() || db.Database.IsSqlite())
        {
            await db.GitLabWebhookEvents
                .Where(x => x.EventUuid == eventUuid)
                .ExecuteUpdateAsync(s => s
                    .SetProperty(x => x.ProcessedAt, (DateTime?)now), ct);
        }
        else
        {
            // EF InMemory provider (used in BackendFactory integration tests) doesn't support ExecuteUpdateAsync.
            var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid, ct);
            if (row is not null)
            {
                row.ProcessedAt = now;
                await db.SaveChangesAsync(ct);
            }
        }
    }

    /// <summary>
    /// Records a handler failure, incrementing <c>failure_count</c>.
    /// When it reaches <see cref="QuarantineThreshold"/>, <c>quarantined_at</c> is set.
    /// Returns the new failure count.
    /// </summary>
    public async Task<int> RecordFailureAsync(string eventUuid, string errorMessage, CancellationToken ct)
    {
        var now = clock.GetUtcNow().UtcDateTime;
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid, ct)
            ?? throw new InvalidOperationException($"gitlab_webhook_events row missing for {eventUuid}");
        row.FailureCount += 1;
        if (row.FailureCount >= QuarantineThreshold)
            row.QuarantinedAt = now;
        await db.SaveChangesAsync(ct);
        return row.FailureCount;
    }

    /// <summary>
    /// Deletes processed rows older than <paramref name="cutoff"/>.
    /// Quarantined rows and pending rows are retained indefinitely.
    /// Spec :10943 — 30-day retention for GitLab (vs 90 days for GitHub).
    /// </summary>
    public async Task<int> DeleteOlderThanAsync(DateTime cutoff, CancellationToken ct)
    {
        if (db.Database.IsNpgsql() || db.Database.IsSqlite())
        {
            return await db.GitLabWebhookEvents
                .Where(x => x.ProcessedAt != null && x.QuarantinedAt == null && x.ReceivedAt < cutoff)
                .ExecuteDeleteAsync(ct);
        }

        // EF InMemory fallback.
        var rows = await db.GitLabWebhookEvents
            .Where(x => x.ProcessedAt != null && x.QuarantinedAt == null && x.ReceivedAt < cutoff)
            .ToListAsync(ct);
        db.GitLabWebhookEvents.RemoveRange(rows);
        await db.SaveChangesAsync(ct);
        return rows.Count;
    }
}

/// <summary>Result of <see cref="GitLabWebhookStore.TryInsertPendingAsync"/>.</summary>
/// <param name="Inserted">True if a new row was inserted; false if duplicate.</param>
/// <param name="ExistingReceivedAt">When duplicate, the original row's received_at (else null).</param>
/// <param name="ExistingQuarantined">When duplicate, whether the existing row is quarantined (failure_count >= threshold).</param>
/// <param name="ExistingProcessed">When duplicate, whether the existing row is already processed (processed_at IS NOT NULL).</param>
public sealed record GitLabIdempotentInsertResult(
    bool Inserted, DateTime? ExistingReceivedAt, bool ExistingQuarantined, bool ExistingProcessed = false);
