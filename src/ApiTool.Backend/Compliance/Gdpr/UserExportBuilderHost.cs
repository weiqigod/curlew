using System.IO.Pipelines;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Storage;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>
/// Background service that processes <see cref="UserExportStatus.Queued"/> rows one at a time.
/// Uses optimistic claim-by-update (WHERE status = 'Queued' AND version = &lt;read_version&gt;) so
/// concurrent ticks cannot double-process the same row — a second tick receives a
/// <see cref="Microsoft.EntityFrameworkCore.DbUpdateConcurrencyException"/> and skips the row.
/// <para>
/// State machine managed by this host: <c>Queued → Building → Ready</c> (or <c>Failed</c>).
/// The <see cref="UserExportStatus.Expired"/> state is set by the GET endpoint when
/// <c>ExpiresAt</c> has elapsed; the builder does not produce that state.
/// </para>
/// </summary>
public sealed class UserExportBuilderHost(
    IServiceScopeFactory scopeFactory,
    IObjectStore objectStore,
    TimeProvider clock,
    ILogger<UserExportBuilderHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    private static readonly TimeSpan DefaultInterval = TimeSpan.FromSeconds(30);
    private static readonly TimeSpan SignedUrlTtl = TimeSpan.FromHours(24);

    private static readonly GdprBundleManifest Manifest = GdprAttributeScanner.Manifest;

    /// <summary>
    /// Processes one queued <see cref="UserExportRequest"/> row if available.
    /// Safe to call from tests directly.
    /// </summary>
    public async Task TickOnceAsync(CancellationToken ct)
    {
        using var scope = scopeFactory.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        // Claim one queued row: transition to Building atomically.
        var request = await db.UserExportRequests
            .Where(r => r.Status == UserExportStatus.Queued)
            .OrderBy(r => r.CreatedAt)
            .FirstOrDefaultAsync(ct);

        if (request is null)
            return;

        request.Status = UserExportStatus.Building;
        // Rotate the version token atomically: the WHERE clause EF generates will match only
        // the Version we read, so a second tick that read the same row will find a stale token
        // and throw DbUpdateConcurrencyException — preventing double-processing.
        request.Version = Guid.NewGuid();
        try
        {
            await db.SaveChangesAsync(ct);
        }
        catch (DbUpdateConcurrencyException)
        {
            // Another tick claimed this row first — skip it this round.
            logger.LogDebug("Concurrent tick claimed export request {Id}; skipping.", request.Id);
            return;
        }

        try
        {
            var now = clock.GetUtcNow().UtcDateTime;
            var bundle = await UserExportBundleAssembler.AssembleAsync(db, Manifest, request.UserId, now, ct);

            var key = $"exports/{request.UserId}/{request.Id}.json";

            // Stream the bundle JSON through a pipe so the serializer feeds bytes directly
            // to the object store without holding the entire JSON in memory at once.
            // This guards against OOM for users with large organization_audit_log result sets.
            var pipe = new Pipe();
            var serializeTask = Task.Run(async () =>
            {
                try
                {
                    await JsonSerializer.SerializeAsync(
                        pipe.Writer.AsStream(),
                        bundle,
                        new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower },
                        ct);
                }
                finally
                {
                    await pipe.Writer.CompleteAsync();
                }
            }, ct);

            await objectStore.PutAsync(key, pipe.Reader.AsStream(), "application/json", ct);
            await serializeTask; // propagate any serializer exception

            request.Status = UserExportStatus.Ready;
            request.ObjectKey = key;
            request.ReadyAt = now;
            request.ExpiresAt = now.Add(SignedUrlTtl);
            await db.SaveChangesAsync(ct);

            logger.LogInformation(
                "Export bundle ready for user {UserId}, request {RequestId}, key {Key}.",
                request.UserId, request.Id, key);
        }
        catch (Exception ex)
        {
            logger.LogError(ex,
                "Export builder failed for request {RequestId}; transitioning to Failed.",
                request.Id);

            // Refresh the tracked entity in case the context is in a bad state.
            db.ChangeTracker.Clear();
            var failed = await db.UserExportRequests.FindAsync([request.Id], ct);
            if (failed is not null)
            {
                failed.Status = UserExportStatus.Failed;
                // Store the exception type only — never the message (PII risk).
                failed.FailureReason = ex.GetType().Name;
                try { await db.SaveChangesAsync(ct); } catch { /* best effort */ }
            }
        }
    }

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var interval = tickInterval ?? DefaultInterval;
        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                await TickOnceAsync(stoppingToken);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
            catch (Exception ex)
            {
                logger.LogError(ex, "Unhandled exception in UserExportBuilderHost tick loop.");
            }

            await Task.Delay(interval, stoppingToken).ConfigureAwait(false);
        }
    }
}
