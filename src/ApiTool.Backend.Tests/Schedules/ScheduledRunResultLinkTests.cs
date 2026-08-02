using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Results;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>
/// Tests verifying the <c>scheduled_runs.result_id</c> FK linkage and idempotent result-post behavior (M16-010).
/// All tests in this class are selected by the observable filter
/// <c>dotnet test … --filter "FullyQualifiedName~ScheduledRunResultLink"</c>.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class ScheduledRunResultLinkTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private Guid _orgGuid;

    private static readonly JsonSerializerOptions JsonOpts = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    public ScheduledRunResultLinkTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"link-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);
        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"link-{_ownerId:N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "LinkTestOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgId = doc.RootElement.GetProperty("id").GetString()!;
        var hex = orgId.StartsWith("org_", StringComparison.Ordinal) ? orgId["org_".Length..] : orgId;
        _orgGuid = Guid.ParseExact(hex, "N");

        await _factory.SeedSubscriptionAsync(_orgGuid, SubscriptionTier.Team);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── List runs: result_id exposure ─────────────────────────────────────────

    [Fact]
    public async Task ListRuns_returns_result_id_after_worker_posts_result()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        // Claim
        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var claim = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        // Post result
        await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{claim!.RunId}/result",
            BuildResultPayload(claim.ClaimToken),
            JsonOpts);

        // List runs
        var orgIdStr = "org_" + _orgGuid.ToString("N");
        var listResp = await _client.GetAsync(
            $"/api/v1/organizations/{orgIdStr}/schedules/{schedule.Name}/runs");
        listResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var doc = JsonDocument.Parse(await listResp.Content.ReadAsStringAsync());
        var runs = doc.RootElement.GetProperty("runs");
        runs.GetArrayLength().Should().Be(1);
        var resultId = runs[0].GetProperty("result_id").GetString();
        resultId.Should().StartWith("res_",
            because: "result_id wire value must be a res_<hex> identifier after the worker posts a result");
    }

    [Fact]
    public async Task ListRuns_returns_null_result_id_for_queued_run()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var orgIdStr = "org_" + _orgGuid.ToString("N");
        var listResp = await _client.GetAsync(
            $"/api/v1/organizations/{orgIdStr}/schedules/{schedule.Name}/runs");
        listResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var doc = JsonDocument.Parse(await listResp.Content.ReadAsStringAsync());
        var runs = doc.RootElement.GetProperty("runs");
        runs.GetArrayLength().Should().Be(1);

        // The global JSON serializer uses WhenWritingNull so null properties are omitted.
        // A missing result_id key is equivalent to null — both mean the run has no result yet.
        var hasResultId = runs[0].TryGetProperty("result_id", out var resultIdProp);
        if (hasResultId)
        {
            resultIdProp.ValueKind.Should().Be(JsonValueKind.Null,
                because: "a queued run has no result yet; result_id must be absent or null");
        }
        // else: property omitted entirely — that is also correct (WhenWritingNull)
    }

    [Fact]
    public async Task ListRuns_returns_null_result_id_for_legacy_completed_row()
    {
        // Insert a ScheduledRun directly with Status=Completed and ResultId=null
        // (simulates a pre-M16-010 legacy row that has no result linkage).
        var schedule = await SeedScheduleAsync(_orgGuid);

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.ScheduledRuns.Add(new ScheduledRun
            {
                Id = Guid.NewGuid(),
                ScheduleId = schedule.Id,
                Status = ScheduledRunStatus.Completed,
                CreatedAt = DateTime.UtcNow,
                CompletedAt = DateTime.UtcNow,
                ResultId = null,   // legacy — no link
            });
            await db.SaveChangesAsync();
        }

        var orgIdStr = "org_" + _orgGuid.ToString("N");
        var listResp = await _client.GetAsync(
            $"/api/v1/organizations/{orgIdStr}/schedules/{schedule.Name}/runs");
        listResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var doc = JsonDocument.Parse(await listResp.Content.ReadAsStringAsync());
        var runs = doc.RootElement.GetProperty("runs");
        runs.GetArrayLength().Should().Be(1);
        var run = runs[0];
        run.GetProperty("status").GetString().Should().Be("completed");

        // The global JSON serializer uses WhenWritingNull so null properties are omitted.
        // A missing result_id key is equivalent to null — both mean no FK is set (legacy row).
        if (run.TryGetProperty("result_id", out var resultIdProp))
        {
            resultIdProp.ValueKind.Should().Be(JsonValueKind.Null,
                because: "legacy completed rows with result_id IS NULL must not throw — null or absent is fine");
        }
        // else: property omitted entirely (WhenWritingNull) — also correct
    }

    // ── Idempotent retry at the HTTP layer ────────────────────────────────────

    [Fact]
    public async Task Result_post_idempotent_retry_with_same_claim_token_returns_200_and_keeps_one_row()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        // Claim
        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var claim = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var url = $"/api/v1/schedules/runs/{claim!.RunId}/result";
        var payload = BuildResultPayload(claim.ClaimToken);

        // First post
        var r1 = await _client.PostAsJsonAsync(url, payload, JsonOpts);
        r1.StatusCode.Should().Be(HttpStatusCode.OK);

        // Second post — same claim_token (idempotent network retry)
        var r2 = await _client.PostAsJsonAsync(url, payload, JsonOpts);
        r2.StatusCode.Should().Be(HttpStatusCode.OK,
            because: "a retry with the same claim_token must return 200 — not 409");

        // Only one results row for this org
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var count = await db.Results.CountAsync(r => r.OrgId == _orgGuid);
        count.Should().Be(1,
            because: "idempotent retry must not insert a second results row");
    }

    // ── ON DELETE SET NULL (entity-level, SQLite TestDb fixture) ─────────────

    [Fact]
    public async Task DeletingResult_sets_scheduled_runs_result_id_to_null()
    {
        // Arrange: set up a fully-linked run via the service layer against a real SQLite TestDb
        // so ON DELETE SET NULL FK behaviour is actually enforced.
        using var scope = TestDb.CreateOpen();
        scope.Db.Database.Migrate();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        scope.Db.Users.Add(new User { Id = userId, Email = $"del-{userId:N}@t.com", CreatedAt = DateTime.UtcNow });
        scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = $"Del-{orgId:N}"[..20],
            Slug = $"del-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        scope.Db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            PermissionsJson = "[]",
            JoinedAt = DateTime.UtcNow,
        });
        var schedule = new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = orgId,
            Name = "del-test",
            CronExpression = "0 2 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = userId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        scope.Db.Schedules.Add(schedule);
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var notifier = new NoopResultIngestedNotifier();
        var resultsSvc = new ResultsService(scope.Db, clock, notifier);
        var execSvc = new ScheduleExecutorService(
            scope.Db, clock, resultsSvc, new FakeScheduleEnvKeyProvider(), NullLogger<ScheduleExecutorService>.Instance);

        // Enqueue + claim
        var run = new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = schedule.Id,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        };
        scope.Db.ScheduledRuns.Add(run);
        await scope.Db.SaveChangesAsync();
        scope.Db.ChangeTracker.Clear();

        var (claimDto, _) = await execSvc.ClaimNextAsync(orgId, "w1", default);
        RunId.TryParse(claimDto!.RunId, out var runId);

        var req = new ScheduleResultRequest
        {
            ClaimToken = claimDto.ClaimToken,
            CollectionName = "del-smoke",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            TriggeredBy = "schedule",
            Items = [],
        };
        await execSvc.SubmitResultAsync(userId, orgId, runId, req, default);

        scope.Db.ChangeTracker.Clear();
        var linked = await scope.Db.ScheduledRuns.FindAsync(run.Id);
        linked!.ResultId.Should().NotBeNull(because: "result FK should be set before deletion");

        // Act: delete the result row
        var result = await scope.Db.Results.FindAsync(linked.ResultId!.Value);
        scope.Db.Results.Remove(result!);
        await scope.Db.SaveChangesAsync();

        // Assert: the scheduled_run row still exists and result_id is null (ON DELETE SET NULL)
        scope.Db.ChangeTracker.Clear();
        var afterDelete = await scope.Db.ScheduledRuns.FindAsync(run.Id);
        afterDelete.Should().NotBeNull(because: "deleting a result must not cascade-delete the scheduled_run row");
        afterDelete!.ResultId.Should().BeNull(because: "ON DELETE SET NULL must clear result_id when the result row is removed");
    }

    // ── OpenAPI schema: result_id must appear ─────────────────────────────────

    [Fact]
    public async Task Swagger_schema_includes_result_id_on_ScheduledRunDto()
    {
        var resp = await _client.GetAsync("/swagger/v1/swagger.json");
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("resultId",
            because: "ScheduledRunDto.ResultId must appear in the OpenAPI schema as 'resultId'");
    }

    // ── Seed helpers ──────────────────────────────────────────────────────────

    private async Task<Schedule> SeedScheduleAsync(Guid orgGuid)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var schedule = new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = orgGuid,
            Name = $"link-{Guid.NewGuid():N}"[..30],
            CronExpression = "0 2 * * *",
            CollectionRef = "smoke.yaml",
            Enabled = true,
            CreatedBy = _ownerId,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        };
        db.Schedules.Add(schedule);
        await db.SaveChangesAsync();
        return schedule;
    }

    private async Task<ScheduledRun> SeedQueuedRunAsync(Guid scheduleId)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var run = new ScheduledRun
        {
            Id = Guid.NewGuid(),
            ScheduleId = scheduleId,
            Status = ScheduledRunStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        };
        db.ScheduledRuns.Add(run);
        await db.SaveChangesAsync();
        return run;
    }

    private static object BuildResultPayload(string claimToken) => new
    {
        claim_token = claimToken,
        collection_name = "smoke",
        run_at = DateTime.UtcNow,
        duration_ms = 500,
        pass_count = 3,
        fail_count = 0,
        skipped_count = 0,
        triggered_by = "schedule",
        items = Array.Empty<object>(),
    };
}
