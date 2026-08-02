using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>HTTP integration tests for schedule-executor endpoints (M16-009).</summary>
[Collection(BackendCollection.Name)]
public sealed class ScheduleExecutorEndpointsTests : IAsyncLifetime
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

    public ScheduleExecutorEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"exec-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);
        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        // Create org and upgrade to Team tier
        var slug = $"exec-{_ownerId:N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "ExecTestOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgId = doc.RootElement.GetProperty("id").GetString()!;
        var hex = orgId.StartsWith("org_", StringComparison.Ordinal) ? orgId["org_".Length..] : orgId;
        _orgGuid = Guid.ParseExact(hex, "N");

        await _factory.SeedSubscriptionAsync(_orgGuid, SubscriptionTier.Team);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── Auth guard ────────────────────────────────────────────────────────────

    [Fact]
    public async Task All_executor_endpoints_return_401_without_bearer()
    {
        var anon = _factory.CreateClient();
        var r1 = await anon.GetAsync("/api/v1/schedules/next-run");
        var r2 = await anon.PostAsJsonAsync("/api/v1/schedules/runs/run_abc123/heartbeat", new { });
        var r3 = await anon.PostAsJsonAsync("/api/v1/schedules/runs/run_abc123/result", new { });

        r1.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        r2.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        r3.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── Tier gate ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task NextRun_returns_402_with_tier_payload_for_free_tier_org()
    {
        // Create a second user with a fresh Free org
        var (freeToken, freeOrgGuid) = await CreateFreeTierOrgAsync();
        var freeClient = MakeClient(freeToken);

        // Seed a queued run in the free org
        var schedule = await SeedScheduleAsync(freeOrgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var resp = await freeClient.GetAsync("/api/v1/schedules/next-run");
        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task Heartbeat_returns_402_for_free_tier_org()
    {
        var (freeToken, _) = await CreateFreeTierOrgAsync();
        var freeClient = MakeClient(freeToken);

        var resp = await freeClient.PostAsJsonAsync(
            "/api/v1/schedules/runs/run_abc123/heartbeat",
            new { claim_token = Guid.NewGuid().ToString() },
            JsonOpts);

        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task Result_returns_402_for_free_tier_org()
    {
        var (freeToken, _) = await CreateFreeTierOrgAsync();
        var freeClient = MakeClient(freeToken);

        var resp = await freeClient.PostAsJsonAsync(
            "/api/v1/schedules/runs/run_abc123/result",
            new
            {
                claim_token = Guid.NewGuid().ToString(),
                collection_name = "smoke",
                run_at = DateTime.UtcNow,
                duration_ms = 100,
                pass_count = 1,
                fail_count = 0,
                skipped_count = 0,
                triggered_by = "schedule",
                items = Array.Empty<object>(),
            },
            JsonOpts);

        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task NextRun_returns_403_when_user_has_no_org()
    {
        // Create a user with a valid JWT but never create an org — no OrganizationMembers row.
        var (noOrgToken, _) = TestTokens.CreateNew($"no-org-{Guid.NewGuid():N}@example.com");
        var noOrgClient = MakeClient(noOrgToken);

        // Trigger user upsert with a harmless authenticated call so the user row exists.
        await noOrgClient.GetAsync("/api/v1/organizations");

        var resp = await noOrgClient.GetAsync("/api/v1/schedules/next-run");

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("no_org_membership");
    }

    // ── 204 No Content when no runs queued ────────────────────────────────────

    [Fact]
    public async Task NextRun_returns_204_when_no_queued_rows()
    {
        var resp = await _client.GetAsync("/api/v1/schedules/next-run");
        resp.StatusCode.Should().Be(HttpStatusCode.NoContent);
    }

    // ── 200 claim happy path ──────────────────────────────────────────────────

    [Fact]
    public async Task NextRun_returns_200_with_run_id_schedule_id_collection_ref_claim_token_deadline()
    {
        var schedule = await SeedScheduleAsync(_orgGuid, collectionRef: "tests/e2e.yaml");
        await SeedQueuedRunAsync(schedule.Id);

        var resp = await _client.GetAsync("/api/v1/schedules/next-run");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await resp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);
        dto.Should().NotBeNull();
        dto!.RunId.Should().StartWith("run_");
        dto.ScheduleId.Should().StartWith("sched_");
        dto.CollectionRef.Should().Be("tests/e2e.yaml");
        dto.ClaimToken.Should().NotBeNullOrEmpty();
        Guid.TryParse(dto.ClaimToken, out _).Should().BeTrue();
        dto.Deadline.Should().BeAfter(DateTime.UtcNow);
    }

    [Fact]
    public async Task NextRun_second_call_returns_204_after_first_claim()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var r1 = await _client.GetAsync("/api/v1/schedules/next-run");
        r1.StatusCode.Should().Be(HttpStatusCode.OK);

        var r2 = await _client.GetAsync("/api/v1/schedules/next-run");
        r2.StatusCode.Should().Be(HttpStatusCode.NoContent, because: "in-flight row must not be handed out twice");
    }

    [Fact]
    public async Task NextRun_transitions_row_to_running_and_persists_claim_token()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        var resp = await _client.GetAsync("/api/v1/schedules/next-run");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Running);
        updated.ClaimToken.Should().NotBeNull();
    }

    // ── Heartbeat ─────────────────────────────────────────────────────────────

    [Fact]
    public async Task Heartbeat_returns_200_with_valid_claim_token()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var hbResp = await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/heartbeat",
            new { claim_token = dto.ClaimToken },
            JsonOpts);

        hbResp.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Heartbeat_returns_409_claim_reaped_when_row_back_to_queued()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        // Simulate reaper — reset the row
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var r = await db.ScheduledRuns.FindAsync(run.Id);
            r!.Status = ScheduledRunStatus.Queued;
            r.ClaimToken = null;
            await db.SaveChangesAsync();
        }

        var hbResp = await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/heartbeat",
            new { claim_token = dto.ClaimToken },
            JsonOpts);

        hbResp.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await hbResp.Content.ReadAsStringAsync();
        body.Should().Contain("claim-reaped");
    }

    [Fact]
    public async Task Heartbeat_returns_409_for_stale_token()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var hbResp = await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/heartbeat",
            new { claim_token = Guid.NewGuid().ToString() },
            JsonOpts);

        hbResp.StatusCode.Should().Be(HttpStatusCode.Conflict);
    }

    // ── Result ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Result_transitions_to_completed_when_fail_count_zero()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var resultResp = await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/result",
            new
            {
                claim_token = dto.ClaimToken,
                collection_name = "smoke",
                run_at = DateTime.UtcNow,
                duration_ms = 500,
                pass_count = 3,
                fail_count = 0,
                skipped_count = 0,
                triggered_by = "schedule",
                items = Array.Empty<object>(),
            },
            JsonOpts);

        resultResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Completed);
    }

    [Fact]
    public async Task Result_transitions_to_failed_when_fail_count_gt_zero()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var resultResp = await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/result",
            new
            {
                claim_token = dto.ClaimToken,
                collection_name = "smoke",
                run_at = DateTime.UtcNow,
                duration_ms = 200,
                pass_count = 1,
                fail_count = 2,
                skipped_count = 0,
                triggered_by = "schedule",
                items = Array.Empty<object>(),
            },
            JsonOpts);

        resultResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.ScheduledRuns.FindAsync(run.Id);
        updated!.Status.Should().Be(ScheduledRunStatus.Failed);
    }

    [Fact]
    public async Task Result_persists_to_results_table_and_sets_result_id_FK()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        await _client.PostAsJsonAsync(
            $"/api/v1/schedules/runs/{dto!.RunId}/result",
            new
            {
                claim_token = dto.ClaimToken,
                collection_name = "smoke",
                run_at = DateTime.UtcNow,
                duration_ms = 100,
                pass_count = 1,
                fail_count = 0,
                skipped_count = 0,
                triggered_by = "schedule",
                items = Array.Empty<object>(),
            },
            JsonOpts);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.ScheduledRuns.FindAsync(run.Id);
        updated!.ResultId.Should().NotBeNull(because: "result FK must be set after successful result submission");

        var result = await db.Results.FindAsync(updated.ResultId!.Value);
        result.Should().NotBeNull();
        result!.CollectionName.Should().Be("smoke");
    }

    [Fact]
    public async Task Result_returns_409_already_completed_when_token_differs()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var firstPayload = new
        {
            claim_token = dto!.ClaimToken,
            collection_name = "smoke",
            run_at = DateTime.UtcNow,
            duration_ms = 100,
            pass_count = 1,
            fail_count = 0,
            skipped_count = 0,
            triggered_by = "schedule",
            items = Array.Empty<object>(),
        };
        var url = $"/api/v1/schedules/runs/{dto.RunId}/result";

        await _client.PostAsJsonAsync(url, firstPayload, JsonOpts);

        // Second post with a DIFFERENT claim_token — must still 409
        var secondPayload = new
        {
            claim_token = Guid.NewGuid().ToString(),   // different token
            collection_name = "smoke",
            run_at = DateTime.UtcNow,
            duration_ms = 100,
            pass_count = 1,
            fail_count = 0,
            skipped_count = 0,
            triggered_by = "schedule",
            items = Array.Empty<object>(),
        };
        var r2 = await _client.PostAsJsonAsync(url, secondPayload, JsonOpts);

        r2.StatusCode.Should().Be(HttpStatusCode.Conflict,
            because: "a terminal row with a mismatched claim_token must remain 409 (not treated as idempotent retry)");
    }

    [Fact]
    public async Task Result_returns_200_idempotent_retry_with_same_claim_token()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        await SeedQueuedRunAsync(schedule.Id);

        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        var dto = await claimResp.Content.ReadFromJsonAsync<NextRunResponse>(JsonOpts);

        var payload = new
        {
            claim_token = dto!.ClaimToken,
            collection_name = "smoke-retry",
            run_at = DateTime.UtcNow,
            duration_ms = 100,
            pass_count = 1,
            fail_count = 0,
            skipped_count = 0,
            triggered_by = "schedule",
            items = Array.Empty<object>(),
        };
        var url = $"/api/v1/schedules/runs/{dto.RunId}/result";

        // First post
        var r1 = await _client.PostAsJsonAsync(url, payload, JsonOpts);
        r1.StatusCode.Should().Be(HttpStatusCode.OK);

        // Second post — same claim_token (idempotent network retry)
        var r2 = await _client.PostAsJsonAsync(url, payload, JsonOpts);
        r2.StatusCode.Should().Be(HttpStatusCode.OK,
            because: "same claim_token retry must return 200 idempotently — not 409");
    }

    // ── ShardReaper integration (via direct DB manipulation) ──────────────────

    [Fact]
    public async Task Reaper_returns_running_row_to_queued_after_5_minutes_no_heartbeat()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        // Claim the run
        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Simulate a stale heartbeat by writing last_heartbeat_at far in the past
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var r = await db.ScheduledRuns.FindAsync(run.Id);
            r!.LastHeartbeatAt = DateTime.UtcNow.AddMinutes(-10);
            await db.SaveChangesAsync();
        }

        // Invoke the reaper directly
        using (var scope = _factory.Services.CreateScope())
        {
            var reaper = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Coordinator.IShardReaper>();
            await reaper.ReapStaleShardsAsync(default);
        }

        // Row must be back to Queued
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var r = await db.ScheduledRuns.FindAsync(run.Id);
            r!.Status.Should().Be(ScheduledRunStatus.Queued,
                because: "reaper must reset stale running row to Queued");
            r.ClaimToken.Should().BeNull();
        }

        // Next-run should claim it again
        var r2 = await _client.GetAsync("/api/v1/schedules/next-run");
        r2.StatusCode.Should().Be(HttpStatusCode.OK, because: "reaped row should be claimable again");
    }

    // ── SchedulerHost stack-up (via direct DB manipulation) ───────────────────

    [Fact]
    public async Task EnqueueDue_skips_when_previous_run_still_running()
    {
        var schedule = await SeedScheduleAsync(_orgGuid);
        var run = await SeedQueuedRunAsync(schedule.Id);

        // Claim the run (status → Running)
        var claimResp = await _client.GetAsync("/api/v1/schedules/next-run");
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Set NextRunAt to past to trigger EnqueueDueAsync
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var s = await db.Schedules.FindAsync(schedule.Id);
            s!.NextRunAt = DateTime.UtcNow.AddMinutes(-1);
            await db.SaveChangesAsync();
        }

        // Run EnqueueDueAsync directly
        using (var scope = _factory.Services.CreateScope())
        {
            var enqueuer = scope.ServiceProvider.GetRequiredService<ISchedulerEnqueuer>();
            await enqueuer.EnqueueDueAsync(default);
        }

        // Should still be exactly 1 run (no stack-up)
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var count = await Microsoft.EntityFrameworkCore.EntityFrameworkQueryableExtensions
                .CountAsync(db.ScheduledRuns.Where(r => r.ScheduleId == schedule.Id));
            count.Should().Be(1, because: "stack-up prevention: no new run when previous is still running");
        }
    }

    // ── Swagger doc ───────────────────────────────────────────────────────────

    [Fact]
    public async Task Swagger_json_lists_schedule_executor_endpoints()
    {
        var resp = await _client.GetAsync("/swagger/v1/swagger.json");
        resp.EnsureSuccessStatusCode();

        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("/api/v1/schedules/next-run",
            because: "next-run endpoint must appear in OpenAPI spec");
        json.Should().Contain("heartbeat",
            because: "heartbeat endpoint must appear in OpenAPI spec");
        json.Should().Contain("result",
            because: "result endpoint must appear in OpenAPI spec");

        // DoD: OpenAPI doc must include the claim_token contract fields from NextRunResponse.
        // ASP.NET Core generates camelCase property names in the OpenAPI schema.
        json.Should().Contain("claimToken",
            because: "NextRunResponse.ClaimToken must appear in the OpenAPI schema as 'claimToken'");
        json.Should().Contain("deadline",
            because: "NextRunResponse.Deadline must appear in the OpenAPI schema");
        json.Should().Contain("envVars",
            because: "NextRunResponse.EnvVars must appear in the OpenAPI schema as 'envVars'");
    }

    // ── Seed helpers ──────────────────────────────────────────────────────────

    private async Task<(string token, Guid orgGuid)> CreateFreeTierOrgAsync()
    {
        var userId = Guid.NewGuid();
        var email = $"free-{userId:N}@example.com";
        var token = TestTokens.Create(userId, email);
        var client = MakeClient(token);

        // Trigger user upsert
        await client.GetAsync("/api/v1/organizations");

        var slug = $"free-{userId:N}"[..20];
        var resp = await client.PostAsJsonAsync("/api/v1/organizations", new { name = "FreeOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgId = doc.RootElement.GetProperty("id").GetString()!;
        var hex = orgId.StartsWith("org_", StringComparison.Ordinal) ? orgId["org_".Length..] : orgId;
        return (token, Guid.ParseExact(hex, "N"));
    }

    private HttpClient MakeClient(string token)
    {
        var c = _factory.CreateClient();
        c.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
        return c;
    }

    private async Task<Schedule> SeedScheduleAsync(Guid orgGuid, string collectionRef = "smoke.yaml")
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var schedule = new Schedule
        {
            Id = Guid.NewGuid(),
            OrgId = orgGuid,
            Name = $"sched-{Guid.NewGuid():N}"[..30],
            CronExpression = "0 2 * * *",
            CollectionRef = collectionRef,
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
}
