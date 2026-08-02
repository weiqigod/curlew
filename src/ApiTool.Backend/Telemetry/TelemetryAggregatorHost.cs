using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// Daily background service (04:00 UTC) that aggregates closed UTC days from
/// <c>telemetry_events</c> into <c>telemetry_daily_aggregates</c>.
/// One aggregate row is produced per <c>(day, event_type)</c>; re-runs are idempotent
/// because already-aggregated days are skipped.
/// </summary>
/// <remarks>
/// Numeric payload fields aggregated: <c>duration_ms</c>, <c>collection_size</c>,
/// <c>request_count</c>, <c>failure_count</c>, <c>success_count</c>.
/// Unknown payload keys are ignored — forward-compat with future CLI versions.
/// </remarks>
public sealed class TelemetryAggregatorHost(
    IServiceScopeFactory scopeFactory,
    IOptions<TelemetryAggregatorOptions> options,
    TimeProvider clock,
    ILogger<TelemetryAggregatorHost> logger,
    TimeSpan? tickInterval = null) : BackgroundService
{
    /// <summary>Numeric payload keys that are aggregated (sum, min, max, avg, n).</summary>
    internal static readonly IReadOnlyList<string> AggregatedNumericKeys =
        ["duration_ms", "collection_size", "request_count", "failure_count", "success_count"];

    private readonly TimeSpan _tickInterval = tickInterval ?? options.Value.TickInterval;
    private readonly int _runHourUtc = options.Value.RunHourUtc;

    /// <summary>
    /// Processes all closed UTC days that have no aggregate row yet.
    /// Safe to call from tests directly (bypasses the daily schedule logic).
    /// </summary>
    public async Task TickOnceAsync(CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var todayUtc = DateOnly.FromDateTime(clock.GetUtcNow().UtcDateTime);

        // Find all distinct (day, event_type) combos in telemetry_events that are:
        //   - from a fully-closed day (< today)
        //   - not yet in telemetry_daily_aggregates
        var pendingDays = await db.TelemetryEvents
            .Where(e => DateOnly.FromDateTime(e.ReceivedAt) < todayUtc)
            .Select(e => new { Day = DateOnly.FromDateTime(e.ReceivedAt), e.EventType })
            .Distinct()
            .ToListAsync(ct);

        // Filter out already-aggregated (day, event_type) pairs.
        var existingKeys = await db.TelemetryDailyAggregates
            .Where(a => a.Day < todayUtc)
            .Select(a => new { a.Day, a.EventType })
            .ToListAsync(ct);

        var existingSet = new HashSet<(DateOnly, string)>(
            existingKeys.Select(k => (k.Day, k.EventType)));

        var toAggregate = pendingDays
            .Where(p => !existingSet.Contains((p.Day, p.EventType)))
            .ToList();

        if (toAggregate.Count == 0)
        {
            logger.LogDebug("TelemetryAggregatorHost: no pending days to aggregate");
            return;
        }

        foreach (var key in toAggregate)
        {
            var events = await db.TelemetryEvents
                .Where(e => DateOnly.FromDateTime(e.ReceivedAt) == key.Day &&
                            e.EventType == key.EventType)
                .Select(e => new { e.InstallId, e.EventPayloadJson })
                .ToListAsync(ct);

            var distinctInstalls = events.Select(e => e.InstallId).Distinct().Count();
            var numericSums = ComputeNumericSums(events.Select(e => e.EventPayloadJson));

            var aggregate = new TelemetryDailyAggregate
            {
                Day = key.Day,
                EventType = key.EventType,
                EventCount = events.Count,
                DistinctInstallCount = distinctInstalls,
                NumericSumsJson = numericSums,
            };

            db.TelemetryDailyAggregates.Add(aggregate);

            try
            {
                await db.SaveChangesAsync(ct);
            }
            catch (DbUpdateException)
            {
                // Another process beat us — safe to skip; the existing row wins.
                db.ChangeTracker.Clear();
                logger.LogDebug(
                    "TelemetryAggregatorHost: skipping ({Day}, {EventType}) — already aggregated by another process",
                    key.Day, key.EventType);
                continue;
            }

            logger.LogInformation(
                "telemetry_aggregated day={Day} event_type={EventType} count={Count} installs={Installs}",
                key.Day, key.EventType, events.Count, distinctInstalls);
        }
    }

    /// <inheritdoc/>
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        // Delay to the next RunHourUtc UTC before starting the loop.
        var now = clock.GetUtcNow();
        var nextRun = now.UtcDateTime.Date.AddHours(_runHourUtc);
        if (nextRun <= now.UtcDateTime)
            nextRun = nextRun.AddDays(1);
        var initialDelay = nextRun - now.UtcDateTime;

        logger.LogInformation(
            "TelemetryAggregatorHost started; first tick at {NextRun:o}",
            nextRun);

        try
        {
            await Task.Delay(initialDelay, stoppingToken).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
        {
            return;
        }

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
                logger.LogError(ex, "TelemetryAggregatorHost tick failed");
            }

            try
            {
                await Task.Delay(_tickInterval, stoppingToken).ConfigureAwait(false);
            }
            catch (OperationCanceledException) when (stoppingToken.IsCancellationRequested)
            {
                break;
            }
        }

        logger.LogInformation("TelemetryAggregatorHost stopped");
    }

    /// <summary>
    /// Computes sum, min, max, avg, and n for each whitelisted numeric key
    /// across all event payload JSON strings. Returns a compact JSON object.
    /// </summary>
    private static string ComputeNumericSums(IEnumerable<string> payloadJsons)
    {
        // key → (sum, min, max, n)
        var accumulators = new Dictionary<string, (double Sum, double Min, double Max, int N)>();

        foreach (var json in payloadJsons)
        {
            if (string.IsNullOrWhiteSpace(json) || json == "{}")
                continue;

            JsonDocument? doc = null;
            try
            {
                doc = JsonDocument.Parse(json);
                foreach (var key in AggregatedNumericKeys)
                {
                    if (!doc.RootElement.TryGetProperty(key, out var prop))
                        continue;

                    double val;
                    if (prop.ValueKind == JsonValueKind.Number)
                        val = prop.GetDouble();
                    else
                        continue;

                    if (accumulators.TryGetValue(key, out var acc))
                        accumulators[key] = (acc.Sum + val, Math.Min(acc.Min, val), Math.Max(acc.Max, val), acc.N + 1);
                    else
                        accumulators[key] = (val, val, val, 1);
                }
            }
            catch (JsonException)
            {
                // Malformed payload — skip; don't crash the aggregator.
            }
            finally
            {
                doc?.Dispose();
            }
        }

        if (accumulators.Count == 0)
            return "{}";

        using var ms = new System.IO.MemoryStream();
        using var writer = new Utf8JsonWriter(ms);
        writer.WriteStartObject();
        foreach (var (key, (sum, min, max, n)) in accumulators)
        {
            writer.WritePropertyName(key);
            writer.WriteStartObject();
            writer.WriteNumber("sum", sum);
            writer.WriteNumber("min", min);
            writer.WriteNumber("max", max);
            writer.WriteNumber("avg", n > 0 ? sum / n : 0.0);
            writer.WriteNumber("n", n);
            writer.WriteEndObject();
        }
        writer.WriteEndObject();
        writer.Flush();
        return System.Text.Encoding.UTF8.GetString(ms.ToArray());
    }
}
