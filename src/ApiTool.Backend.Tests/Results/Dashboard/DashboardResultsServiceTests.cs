using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results.Dashboard;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Results.Dashboard;

/// <summary>Unit tests for <see cref="DashboardResultsService"/> against SQLite in-memory.</summary>
public sealed class DashboardResultsServiceTests
{
    // ── Helpers ──────────────────────────────────────────────────────────────

    private static readonly DateTimeOffset FixedNow =
        new(2026, 5, 12, 12, 0, 0, TimeSpan.Zero);

    private static async Task<(TestDbScope scope, AppDbContext db, DashboardResultsService svc, Guid orgId, Guid userId)>
        BuildAsync(DateTimeOffset? now = null)
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var clock = new FakeClock(now ?? FixedNow);
        var svc = new DashboardResultsService(db, clock);

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"dash-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "DashOrg",
            Slug = $"dashorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        return (scope, db, svc, orgId, userId);
    }

    /// <summary>Seeds a <see cref="Result"/> with the given parameters.</summary>
    private static async Task<Guid> SeedResultAsync(
        AppDbContext db,
        Guid orgId,
        Guid userId,
        DateTime createdAt,
        long durationMs = 100,
        int passCount = 1,
        int failCount = 0,
        int skippedCount = 0)
    {
        var resultId = Guid.NewGuid();
        db.Results.Add(new Result
        {
            Id = resultId,
            OrgId = orgId,
            UploadedBy = userId,
            CollectionName = "test",
            RunAt = createdAt,
            DurationMs = durationMs,
            PassCount = passCount,
            FailCount = failCount,
            SkippedCount = skippedCount,
            CreatedAt = createdAt,
        });
        await db.SaveChangesAsync();
        return resultId;
    }

    private static async Task SeedItemAsync(
        AppDbContext db,
        Guid resultId,
        int ordinal,
        ResultStatus status,
        string? method = null,
        string? pathTemplate = null)
    {
        db.ResultItems.Add(new ResultItem
        {
            Id = Guid.NewGuid(),
            ResultId = resultId,
            Ordinal = ordinal,
            Name = $"item-{ordinal}",
            Status = status,
            DurationMs = 50,
            Method = method,
            PathTemplate = pathTemplate,
        });
        await db.SaveChangesAsync();
    }

    // ── STATS tests ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Stats_returns_zero_totals_and_empty_trend_for_empty_org()
    {
        var (scope, _, svc, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.ThirtyDays, default);

            Assert.Equal("30d", resp.Window);
            Assert.Equal(0, resp.Totals.Runs);
            Assert.Equal(0, resp.Totals.PassCount);
            Assert.Equal(0, resp.Totals.FailCount);
            Assert.Equal(0, resp.Totals.SkippedCount);
            Assert.Equal(0.0, resp.Totals.PassRate);
            Assert.Equal(0L, resp.Totals.AvgDurationMs);
            Assert.Equal(0L, resp.Totals.P50DurationMs);
            Assert.Equal(0L, resp.Totals.P95DurationMs);
            Assert.Empty(resp.Trend);
        }
    }

    [Fact]
    public async Task Stats_aggregates_runs_pass_fail_skipped_over_window()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-5);
            await SeedResultAsync(db, orgId, userId, within, passCount: 3, failCount: 1, skippedCount: 0);
            await SeedResultAsync(db, orgId, userId, within.AddHours(2), passCount: 2, failCount: 0, skippedCount: 1);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.SevenDays, default);

            Assert.Equal(2, resp.Totals.Runs);
            Assert.Equal(5, resp.Totals.PassCount);
            Assert.Equal(1, resp.Totals.FailCount);
            Assert.Equal(1, resp.Totals.SkippedCount);
        }
    }

    [Fact]
    public async Task Stats_pass_rate_is_pass_over_pass_plus_fail()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);
            // 3 pass, 1 fail, 1 skipped → pass_rate = 3/(3+1) = 0.75
            await SeedResultAsync(db, orgId, userId, within, passCount: 3, failCount: 1, skippedCount: 1);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.SevenDays, default);

            Assert.Equal(0.75, resp.Totals.PassRate, precision: 4);
        }
    }

    [Fact]
    public async Task Stats_avg_computed_from_run_duration()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);
            await SeedResultAsync(db, orgId, userId, within, durationMs: 100);
            await SeedResultAsync(db, orgId, userId, within.AddHours(1), durationMs: 200);
            await SeedResultAsync(db, orgId, userId, within.AddHours(2), durationMs: 300);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.SevenDays, default);

            // avg = (100+200+300)/3 = 200
            Assert.Equal(200L, resp.Totals.AvgDurationMs);
            // p50 of [100,200,300] = 200
            Assert.Equal(200L, resp.Totals.P50DurationMs);
            // p95 of [100,200,300]: PERCENTILE_CONT(0.95) interpolated.
            // h = 0.95 * (3-1) = 1.9 → lower index 1 (200), upper index 2 (300), fraction 0.9
            // result = 200 + 0.9*(300-200) = 290
            Assert.Equal(290L, resp.Totals.P95DurationMs);
        }
    }

    [Fact]
    public async Task Stats_excludes_rows_older_than_window_start()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var outside = FixedNow.UtcDateTime.AddDays(-31); // outside 30d window
            var inside  = FixedNow.UtcDateTime.AddDays(-5);
            await SeedResultAsync(db, orgId, userId, outside, passCount: 10, failCount: 10);
            await SeedResultAsync(db, orgId, userId, inside, passCount: 1, failCount: 0);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.ThirtyDays, default);

            Assert.Equal(1, resp.Totals.Runs);
            Assert.Equal(1, resp.Totals.PassCount);
            Assert.Equal(0, resp.Totals.FailCount);
        }
    }

    [Fact]
    public async Task Stats_excludes_rows_from_other_orgs()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            // Other org with lots of results
            var otherUserId = Guid.NewGuid();
            var otherOrgId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherUserId, Email = $"other-{otherUserId:N}@test.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = otherOrgId,
                Name = "OtherOrg",
                Slug = $"otherorg-{otherOrgId:N}"[..20],
                OwnerId = otherUserId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var within = FixedNow.UtcDateTime.AddDays(-1);
            await SeedResultAsync(db, otherOrgId, otherUserId, within, passCount: 100, failCount: 50);
            await SeedResultAsync(db, orgId, userId, within, passCount: 2, failCount: 0);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.ThirtyDays, default);

            Assert.Equal(1, resp.Totals.Runs);
            Assert.Equal(2, resp.Totals.PassCount);
            Assert.Equal(0, resp.Totals.FailCount);
        }
    }

    [Fact]
    public async Task Stats_trend_groups_by_utc_calendar_date_sparse()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            // Two results on day -2, one result on day -5, nothing on other days
            var day2 = FixedNow.UtcDateTime.AddDays(-2).Date.AddHours(9);
            var day5 = FixedNow.UtcDateTime.AddDays(-5).Date.AddHours(10);
            await SeedResultAsync(db, orgId, userId, day2, passCount: 1, failCount: 0);
            await SeedResultAsync(db, orgId, userId, day2.AddHours(3), passCount: 0, failCount: 1);
            await SeedResultAsync(db, orgId, userId, day5, passCount: 1, failCount: 1);

            var resp = await svc.GetStatsAsync(orgId, DashboardWindow.SevenDays, default);

            Assert.Equal(2, resp.Trend.Count); // only 2 active days
            var sorted = resp.Trend.OrderBy(t => t.Date).ToList();
            Assert.Equal(day5.ToString("yyyy-MM-dd"), sorted[0].Date);
            Assert.Equal(1, sorted[0].Runs);
            Assert.Equal(day2.ToString("yyyy-MM-dd"), sorted[1].Date);
            Assert.Equal(2, sorted[1].Runs);
            Assert.Equal(1, sorted[1].PassCount);
            Assert.Equal(1, sorted[1].FailCount);
        }
    }

    [Theory]
    [InlineData(DashboardWindow.SevenDays,  7)]
    [InlineData(DashboardWindow.ThirtyDays, 30)]
    [InlineData(DashboardWindow.NinetyDays, 90)]
    public async Task Stats_window_start_is_now_minus_n_days(DashboardWindow w, int n)
    {
        var (scope, _, svc, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var resp = await svc.GetStatsAsync(orgId, w, default);
            var expectedStart = FixedNow.UtcDateTime.AddDays(-n);
            var expectedEnd   = FixedNow.UtcDateTime;

            Assert.Equal(expectedStart, resp.WindowStart, TimeSpan.FromSeconds(1));
            Assert.Equal(expectedEnd,   resp.WindowEnd,   TimeSpan.FromSeconds(1));
        }
    }

    // ── FAILURES tests ────────────────────────────────────────────────────────

    [Fact]
    public async Task Failures_groups_by_method_and_path_template()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);
            var r1 = await SeedResultAsync(db, orgId, userId, within, failCount: 1);
            var r2 = await SeedResultAsync(db, orgId, userId, within.AddHours(1), failCount: 2);

            await SeedItemAsync(db, r1, 0, ResultStatus.Failed, "GET", "/users/{id}");
            await SeedItemAsync(db, r2, 0, ResultStatus.Failed, "GET", "/users/{id}");
            await SeedItemAsync(db, r2, 1, ResultStatus.Failed, "POST", "/orders");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Equal(2, resp.Items.Count);
            // Sorted by failure_count DESC
            Assert.Equal("/users/{id}", resp.Items[0].PathTemplate);
            Assert.Equal("GET", resp.Items[0].Method);
            Assert.Equal(2, resp.Items[0].FailureCount);
            Assert.Equal("/orders", resp.Items[1].PathTemplate);
            Assert.Equal(1, resp.Items[1].FailureCount);
        }
    }

    [Fact]
    public async Task Failures_excludes_items_with_null_path_template()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);
            var r = await SeedResultAsync(db, orgId, userId, within, failCount: 2);
            // Legacy item — no path_template
            await SeedItemAsync(db, r, 0, ResultStatus.Failed, method: null, pathTemplate: null);
            // Valid item
            await SeedItemAsync(db, r, 1, ResultStatus.Failed, "GET", "/api/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Single(resp.Items);
            Assert.Equal("/api/{id}", resp.Items[0].PathTemplate);
        }
    }

    [Fact]
    public async Task Failures_excludes_items_with_status_passed_or_skipped()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);
            var r = await SeedResultAsync(db, orgId, userId, within, passCount: 1, skippedCount: 1, failCount: 1);
            await SeedItemAsync(db, r, 0, ResultStatus.Passed, "GET", "/users/{id}");
            await SeedItemAsync(db, r, 1, ResultStatus.Skipped, "GET", "/users/{id}");
            await SeedItemAsync(db, r, 2, ResultStatus.Failed, "GET", "/users/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Single(resp.Items);
            Assert.Equal(1, resp.Items[0].FailureCount);
        }
    }

    [Fact]
    public async Task Failures_limit_clamps_at_50_and_sets_limit_clamped()
    {
        var (scope, _, svc, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 200, default);
            Assert.Equal(50, resp.Limit);
            Assert.True(resp.LimitClamped);
        }
    }

    [Fact]
    public async Task Failures_limit_below_50_does_not_set_limit_clamped()
    {
        var (scope, _, svc, orgId, _) = await BuildAsync();
        await using (scope)
        {
            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);
            Assert.Equal(10, resp.Limit);
            Assert.False(resp.LimitClamped);
        }
    }

    [Fact]
    public async Task Failures_first_last_seen_at_track_min_max_created_at()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var t1 = FixedNow.UtcDateTime.AddDays(-3);
            var t2 = FixedNow.UtcDateTime.AddDays(-1);
            var r1 = await SeedResultAsync(db, orgId, userId, t1, failCount: 1);
            var r2 = await SeedResultAsync(db, orgId, userId, t2, failCount: 1);
            await SeedItemAsync(db, r1, 0, ResultStatus.Failed, "GET", "/orgs/{id}");
            await SeedItemAsync(db, r2, 0, ResultStatus.Failed, "GET", "/orgs/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Single(resp.Items);
            Assert.Equal(t1, resp.Items[0].FirstSeenAt, TimeSpan.FromSeconds(1));
            Assert.Equal(t2, resp.Items[0].LastSeenAt, TimeSpan.FromSeconds(1));
        }
    }

    [Fact]
    public async Task Failures_sample_run_ids_are_three_most_recent_parent_result_ids()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var t1 = FixedNow.UtcDateTime.AddDays(-5);
            var t2 = FixedNow.UtcDateTime.AddDays(-3);
            var t3 = FixedNow.UtcDateTime.AddDays(-2);
            var t4 = FixedNow.UtcDateTime.AddDays(-1);

            var r1 = await SeedResultAsync(db, orgId, userId, t1, failCount: 1);
            var r2 = await SeedResultAsync(db, orgId, userId, t2, failCount: 1);
            var r3 = await SeedResultAsync(db, orgId, userId, t3, failCount: 1);
            var r4 = await SeedResultAsync(db, orgId, userId, t4, failCount: 1);

            await SeedItemAsync(db, r1, 0, ResultStatus.Failed, "GET", "/users/{id}");
            await SeedItemAsync(db, r2, 0, ResultStatus.Failed, "GET", "/users/{id}");
            await SeedItemAsync(db, r3, 0, ResultStatus.Failed, "GET", "/users/{id}");
            await SeedItemAsync(db, r4, 0, ResultStatus.Failed, "GET", "/users/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Single(resp.Items);
            var ids = resp.Items[0].SampleRunIds;
            Assert.Equal(3, ids.Count);
            // Should be the 3 most recent: r4, r3, r2 (r1 is oldest)
            Assert.Contains(ids, id => id.Contains(r4.ToString("N")));
            Assert.Contains(ids, id => id.Contains(r3.ToString("N")));
            Assert.Contains(ids, id => id.Contains(r2.ToString("N")));
            Assert.DoesNotContain(ids, id => id.Contains(r1.ToString("N")));
        }
    }

    [Fact]
    public async Task Failures_excludes_rows_from_other_orgs()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var otherUserId = Guid.NewGuid();
            var otherOrgId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherUserId, Email = $"other2-{otherUserId:N}@test.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = otherOrgId,
                Name = "OtherOrg2",
                Slug = $"other2org-{otherOrgId:N}"[..20],
                OwnerId = otherUserId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var within = FixedNow.UtcDateTime.AddDays(-1);
            var rOther = await SeedResultAsync(db, otherOrgId, otherUserId, within, failCount: 10);
            await SeedItemAsync(db, rOther, 0, ResultStatus.Failed, "GET", "/sensitive/{id}");

            var rOwn = await SeedResultAsync(db, orgId, userId, within, failCount: 1);
            await SeedItemAsync(db, rOwn, 0, ResultStatus.Failed, "GET", "/my/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Single(resp.Items);
            Assert.Equal("/my/{id}", resp.Items[0].PathTemplate);
        }
    }

    [Fact]
    public async Task Failures_sorted_by_failure_count_desc_then_path_template_asc()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var within = FixedNow.UtcDateTime.AddDays(-1);

            // Two groups with equal failure_count — secondary sort should be PathTemplate ASC.
            var r1 = await SeedResultAsync(db, orgId, userId, within, failCount: 1);
            var r2 = await SeedResultAsync(db, orgId, userId, within.AddHours(1), failCount: 1);

            await SeedItemAsync(db, r1, 0, ResultStatus.Failed, "GET", "/zebra/{id}");
            await SeedItemAsync(db, r2, 0, ResultStatus.Failed, "GET", "/apple/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.SevenDays, 10, default);

            Assert.Equal(2, resp.Items.Count);
            // Both have failure_count == 1; alphabetically /apple/{id} < /zebra/{id}
            Assert.Equal("/apple/{id}", resp.Items[0].PathTemplate);
            Assert.Equal("/zebra/{id}", resp.Items[1].PathTemplate);
        }
    }

    [Fact]
    public async Task Failures_excludes_items_outside_window()
    {
        var (scope, db, svc, orgId, userId) = await BuildAsync();
        await using (scope)
        {
            var outside = FixedNow.UtcDateTime.AddDays(-31);
            var inside  = FixedNow.UtcDateTime.AddDays(-1);

            var rOld = await SeedResultAsync(db, orgId, userId, outside, failCount: 5);
            await SeedItemAsync(db, rOld, 0, ResultStatus.Failed, "GET", "/old/{id}");

            var rNew = await SeedResultAsync(db, orgId, userId, inside, failCount: 1);
            await SeedItemAsync(db, rNew, 0, ResultStatus.Failed, "GET", "/new/{id}");

            var resp = await svc.GetFailuresAsync(orgId, DashboardWindow.ThirtyDays, 10, default);

            Assert.Single(resp.Items);
            Assert.Equal("/new/{id}", resp.Items[0].PathTemplate);
        }
    }
}
