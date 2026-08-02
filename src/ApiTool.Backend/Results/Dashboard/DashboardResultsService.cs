using System.Data;
using System.Data.Common;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>
/// Aggregation queries backing <c>GET /results/stats</c> and <c>GET /results/failures</c>.
/// <para>
/// Provider branching: when running against Postgres (<see cref="DatabaseFacadeExtensions.IsNpgsql"/>),
/// native SQL with <c>PERCENTILE_CONT</c> and <c>DATE_TRUNC</c> is used so the query is entirely
/// server-side and satisfies the 800 ms latency SLA on large datasets (behavior #7).
/// All other providers (SQLite in tests) fall back to an equivalent C# in-memory computation
/// that exercises exactly the same logic against a migrated SQLite schema.
/// </para>
/// </summary>
public sealed class DashboardResultsService(AppDbContext db, TimeProvider clock)
{
    /// <summary>Default sample-run-ids per failure group (per spec).</summary>
    public const int DefaultSampleRunIds = 3;

    /// <summary>Max failures-endpoint limit (spec: clamps above 50).</summary>
    public const int MaxFailuresLimit = 50;

    /// <summary>Default failures-endpoint limit (spec: 10).</summary>
    public const int DefaultFailuresLimit = 10;

    // ── stats ────────────────────────────────────────────────────────────────

    /// <summary>
    /// Computes totals + per-day trend for the given org and window. Returns zero-filled
    /// totals and empty trend when no rows exist in the window (per behavior #8 — not 404).
    /// </summary>
    public async Task<StatsResponse> GetStatsAsync(
        Guid orgId, DashboardWindow window, CancellationToken ct)
    {
        var now         = clock.GetUtcNow().UtcDateTime;
        var windowStart = now - window.ToTimeSpan();
        var windowWire  = window.ToWire();

        if (db.Database.IsNpgsql())
            return await GetStatsPostgresAsync(orgId, windowWire, windowStart, now, ct);

        return await GetStatsFallbackAsync(orgId, windowWire, windowStart, now, ct);
    }

    // ── failures ─────────────────────────────────────────────────────────────

    /// <summary>
    /// Returns top-N failure groups by failure count over the window.
    /// Filters out items where <see cref="ResultItem.PathTemplate"/> is null
    /// (legacy/non-HTTP rows).
    /// </summary>
    public async Task<FailuresResponse> GetFailuresAsync(
        Guid orgId, DashboardWindow window, int requestedLimit, CancellationToken ct)
    {
        var now         = clock.GetUtcNow().UtcDateTime;
        var windowStart = now - window.ToTimeSpan();
        var windowWire  = window.ToWire();

        var limitClamped = requestedLimit > MaxFailuresLimit;
        var limit = limitClamped
            ? MaxFailuresLimit
            : Math.Max(1, requestedLimit);

        if (db.Database.IsNpgsql())
            return await GetFailuresPostgresAsync(orgId, windowWire, windowStart, now, limit, limitClamped, ct);

        return await GetFailuresFallbackAsync(orgId, windowWire, windowStart, now, limit, limitClamped, ct);
    }

    // ── Postgres native-SQL paths ─────────────────────────────────────────────

    /// <summary>
    /// Postgres stats path: all aggregation — including <c>PERCENTILE_CONT</c> and
    /// <c>DATE_TRUNC('day', …)</c> for trend — is computed entirely in the database.
    /// The <c>(org_id, created_at)</c> index on <c>results</c> is used for both the
    /// totals scan and the trend grouping.
    /// </summary>
    private async Task<StatsResponse> GetStatsPostgresAsync(
        Guid orgId, string windowWire, DateTime windowStart, DateTime now, CancellationToken ct)
    {
        var conn = db.Database.GetDbConnection();
        if (conn.State != ConnectionState.Open)
            await conn.OpenAsync(ct);

        // ── totals ────────────────────────────────────────────────────────────
        StatsTotals totals;
        await using (var cmd = conn.CreateCommand())
        {
            cmd.CommandText = @"
                SELECT
                    COUNT(*)                                                            AS runs,
                    COALESCE(SUM(pass_count), 0)                                       AS pass_count,
                    COALESCE(SUM(fail_count), 0)                                       AS fail_count,
                    COALESCE(SUM(skipped_count), 0)                                    AS skipped_count,
                    CASE
                        WHEN COALESCE(SUM(pass_count), 0) + COALESCE(SUM(fail_count), 0) = 0 THEN 0.0
                        ELSE COALESCE(SUM(pass_count), 0)::float
                             / (COALESCE(SUM(pass_count), 0) + COALESCE(SUM(fail_count), 0))::float
                    END                                                                AS pass_rate,
                    COALESCE(ROUND(AVG(duration_ms)), 0)                               AS avg_duration_ms,
                    COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY duration_ms), 0) AS p50_duration_ms,
                    COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY duration_ms), 0) AS p95_duration_ms
                FROM results
                WHERE org_id = @orgId
                  AND created_at >= @windowStart
                  AND created_at <= @now";

            AddParam(cmd, "@orgId",       orgId,       DbType.Guid);
            AddParam(cmd, "@windowStart", windowStart, DbType.DateTime);
            AddParam(cmd, "@now",         now,         DbType.DateTime);

            await using var reader = await cmd.ExecuteReaderAsync(ct);
            await reader.ReadAsync(ct);
            totals = ReadStatsTotals(reader);
        }

        // ── trend ─────────────────────────────────────────────────────────────
        List<TrendEntry> trend;
        await using (var cmd = conn.CreateCommand())
        {
            cmd.CommandText = @"
                SELECT
                    DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')::date::text AS date,
                    COUNT(*)                                                       AS runs,
                    COALESCE(SUM(pass_count), 0)                                  AS pass_count,
                    COALESCE(SUM(fail_count), 0)                                  AS fail_count,
                    CASE
                        WHEN COALESCE(SUM(pass_count), 0) + COALESCE(SUM(fail_count), 0) = 0 THEN 0.0
                        ELSE COALESCE(SUM(pass_count), 0)::float
                             / (COALESCE(SUM(pass_count), 0) + COALESCE(SUM(fail_count), 0))::float
                    END                                                            AS pass_rate,
                    COALESCE(ROUND(AVG(duration_ms)), 0)                          AS avg_duration_ms
                FROM results
                WHERE org_id = @orgId
                  AND created_at >= @windowStart
                  AND created_at <= @now
                GROUP BY DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')
                ORDER BY date ASC";

            AddParam(cmd, "@orgId",       orgId,       DbType.Guid);
            AddParam(cmd, "@windowStart", windowStart, DbType.DateTime);
            AddParam(cmd, "@now",         now,         DbType.DateTime);

            await using var reader = await cmd.ExecuteReaderAsync(ct);
            trend = [];
            while (await reader.ReadAsync(ct))
                trend.Add(ReadTrendEntry(reader));
        }

        return new StatsResponse(windowWire, windowStart, now, totals, trend);
    }

    /// <summary>
    /// Postgres failures path: GROUP BY + aggregation and the top-3 sample run ids are
    /// all computed server-side using a <c>LATERAL</c> subquery.
    /// </summary>
    private async Task<FailuresResponse> GetFailuresPostgresAsync(
        Guid orgId, string windowWire, DateTime windowStart, DateTime now,
        int limit, bool limitClamped, CancellationToken ct)
    {
        var conn = db.Database.GetDbConnection();
        if (conn.State != ConnectionState.Open)
            await conn.OpenAsync(ct);

        List<FailureGroup> groups;
        await using var cmd = conn.CreateCommand();
        cmd.CommandText = @"
            SELECT
                i.method,
                i.path_template,
                COUNT(*)                   AS failure_count,
                MIN(r.created_at)          AS first_seen_at,
                MAX(r.created_at)          AS last_seen_at,
                ARRAY(
                    SELECT r2.id::text
                    FROM result_items i2
                    JOIN results r2 ON r2.id = i2.result_id
                    WHERE i2.method = i.method
                      AND i2.path_template = i.path_template
                      AND r2.org_id = @orgId
                      AND r2.created_at >= @windowStart
                      AND r2.created_at <= @now
                      AND i2.status = 'Failed'
                    ORDER BY r2.created_at DESC
                    LIMIT 3
                ) AS sample_run_ids
            FROM result_items i
            JOIN results r ON r.id = i.result_id
            WHERE r.org_id = @orgId
              AND r.created_at >= @windowStart
              AND r.created_at <= @now
              AND i.status = 'Failed'
              AND i.path_template IS NOT NULL
              AND i.method IS NOT NULL
            GROUP BY i.method, i.path_template
            ORDER BY failure_count DESC, i.path_template ASC
            LIMIT @limit";

        AddParam(cmd, "@orgId",       orgId,       DbType.Guid);
        AddParam(cmd, "@windowStart", windowStart, DbType.DateTime);
        AddParam(cmd, "@now",         now,         DbType.DateTime);
        AddParam(cmd, "@limit",       limit,       DbType.Int32);

        await using var reader = await cmd.ExecuteReaderAsync(ct);
        groups = [];
        while (await reader.ReadAsync(ct))
        {
            var method       = reader.GetString(0);
            var pathTemplate = reader.GetString(1);
            var failureCount = (int)reader.GetInt64(2);
            var firstSeenAt  = reader.GetDateTime(3);
            var lastSeenAt   = reader.GetDateTime(4);

            // ARRAY(...) returns string[] via Npgsql
            var rawIds = reader.GetValue(5);
            var sampleRunIds = rawIds is string[] arr
                ? arr.Select(id => Guid.TryParse(id, out var g) ? ResultId.Format(g) : id).ToList()
                : [];

            groups.Add(new FailureGroup(method, pathTemplate, failureCount, firstSeenAt, lastSeenAt, sampleRunIds));
        }

        return new FailuresResponse(windowWire, limit, limitClamped, groups);
    }

    // ── Fallback (SQLite / tests) paths ───────────────────────────────────────

    /// <summary>
    /// Non-Postgres stats path: materialises result headers and computes totals, percentiles,
    /// and trend grouping in C#. Semantically equivalent to the Postgres path.
    /// Used by SQLite-backed unit/integration tests.
    /// </summary>
    private async Task<StatsResponse> GetStatsFallbackAsync(
        Guid orgId, string windowWire, DateTime windowStart, DateTime now, CancellationToken ct)
    {
        var results = await db.Results
            .Where(r => r.OrgId == orgId && r.CreatedAt >= windowStart && r.CreatedAt <= now)
            .ToListAsync(ct);

        if (results.Count == 0)
        {
            return new StatsResponse(
                Window: windowWire,
                WindowStart: windowStart,
                WindowEnd: now,
                Totals: new StatsTotals(0, 0, 0, 0, 0.0, 0L, 0L, 0L),
                Trend: []);
        }

        var totalRuns    = results.Count;
        var totalPass    = results.Sum(r => r.PassCount);
        var totalFail    = results.Sum(r => r.FailCount);
        var totalSkipped = results.Sum(r => r.SkippedCount);
        var passRate     = totalPass + totalFail > 0
            ? (double)totalPass / (totalPass + totalFail)
            : 0.0;
        var avgDuration = (long)Math.Round(results.Average(r => (double)r.DurationMs));

        var sorted = results.Select(r => r.DurationMs).Order().ToList();
        var p50 = Percentile(sorted, 0.50);
        var p95 = Percentile(sorted, 0.95);

        var totals = new StatsTotals(totalRuns, totalPass, totalFail, totalSkipped,
            passRate, avgDuration, p50, p95);

        var trend = results
            .GroupBy(r => r.CreatedAt.Date.ToString("yyyy-MM-dd"))
            .Select(g => new TrendEntry(
                Date: g.Key,
                Runs: g.Count(),
                PassCount: g.Sum(r => r.PassCount),
                FailCount: g.Sum(r => r.FailCount),
                PassRate: g.Sum(r => r.PassCount) + g.Sum(r => r.FailCount) > 0
                    ? (double)g.Sum(r => r.PassCount) / (g.Sum(r => r.PassCount) + g.Sum(r => r.FailCount))
                    : 0.0,
                AvgDurationMs: (long)Math.Round(g.Average(r => (double)r.DurationMs))))
            .OrderBy(t => t.Date)
            .ToList();

        return new StatsResponse(windowWire, windowStart, now, totals, trend);
    }

    /// <summary>
    /// Non-Postgres failures path: materialises matching item+result rows and groups in C#.
    /// Semantically equivalent to the Postgres path. Used by SQLite-backed tests.
    /// </summary>
    private async Task<FailuresResponse> GetFailuresFallbackAsync(
        Guid orgId, string windowWire, DateTime windowStart, DateTime now,
        int limit, bool limitClamped, CancellationToken ct)
    {
        var query =
            from item in db.ResultItems
            join result in db.Results on item.ResultId equals result.Id
            where result.OrgId == orgId
               && result.CreatedAt >= windowStart
               && result.CreatedAt <= now
               && item.Status == ResultStatus.Failed
               && item.PathTemplate != null
               && item.Method != null
            select new { item, result };

        var rows = await query.ToListAsync(ct);

        var groups = rows
            .GroupBy(x => new { Method = x.item.Method!, PathTemplate = x.item.PathTemplate! })
            .Select(g =>
            {
                var sampleIds = g
                    .OrderByDescending(x => x.result.CreatedAt)
                    .Select(x => x.result.Id)
                    .Distinct()
                    .Take(DefaultSampleRunIds)
                    .Select(id => ResultId.Format(id))
                    .ToList();

                return new FailureGroup(
                    Method: g.Key.Method,
                    PathTemplate: g.Key.PathTemplate,
                    FailureCount: g.Count(),
                    FirstSeenAt: g.Min(x => x.result.CreatedAt),
                    LastSeenAt: g.Max(x => x.result.CreatedAt),
                    SampleRunIds: sampleIds);
            })
            .OrderByDescending(f => f.FailureCount)
            .ThenBy(f => f.PathTemplate)
            .Take(limit)
            .ToList();

        return new FailuresResponse(windowWire, limit, limitClamped, groups);
    }

    // ── reader helpers ────────────────────────────────────────────────────────

    private static StatsTotals ReadStatsTotals(DbDataReader reader)
    {
        var runs       = (int)reader.GetInt64(0);
        var passCount  = (int)reader.GetInt64(1);
        var failCount  = (int)reader.GetInt64(2);
        var skipped    = (int)reader.GetInt64(3);
        var passRate   = reader.GetDouble(4);
        var avgMs      = (long)reader.GetDouble(5);
        var p50Ms      = (long)reader.GetDouble(6);
        var p95Ms      = (long)reader.GetDouble(7);
        return new StatsTotals(runs, passCount, failCount, skipped, passRate, avgMs, p50Ms, p95Ms);
    }

    private static TrendEntry ReadTrendEntry(DbDataReader reader)
    {
        var date       = reader.GetString(0);
        var runs       = (int)reader.GetInt64(1);
        var passCount  = (int)reader.GetInt64(2);
        var failCount  = (int)reader.GetInt64(3);
        var passRate   = reader.GetDouble(4);
        var avgMs      = (long)reader.GetDouble(5);
        return new TrendEntry(date, runs, passCount, failCount, passRate, avgMs);
    }

    // ── ADO.NET helper ────────────────────────────────────────────────────────

    private static void AddParam(DbCommand cmd, string name, object value, DbType type)
    {
        var p = cmd.CreateParameter();
        p.ParameterName = name;
        p.Value = value;
        p.DbType = type;
        cmd.Parameters.Add(p);
    }

    // ── percentile helper (in-memory, mirrors PERCENTILE_CONT in Postgres) ────

    /// <summary>
    /// Computes PERCENTILE_CONT(p) on an already-sorted list of values.
    /// Returns 0 for empty lists.
    /// </summary>
    private static long Percentile(IList<long> sorted, double p)
    {
        if (sorted.Count == 0) return 0;
        if (sorted.Count == 1) return sorted[0];

        var h = (sorted.Count - 1) * p;
        var lower = (int)Math.Floor(h);
        var upper = (int)Math.Ceiling(h);
        if (lower == upper) return sorted[lower];
        return (long)Math.Round(sorted[lower] + (h - lower) * (sorted[upper] - sorted[lower]));
    }
}
