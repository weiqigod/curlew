using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>HTTP integration tests for the schedules endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SchedulesEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;
    private Guid _orgGuid;

    public SchedulesEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"sched-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"scheds-{_ownerId:N}"[..20];
        var body = new { name = "SchedulesTestOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;

        // Parse the wire-format org id to a Guid for seeding member rows
        var hexPart = _orgId.StartsWith("org_", StringComparison.Ordinal)
            ? _orgId["org_".Length..]
            : _orgId;
        _orgGuid = Guid.ParseExact(hexPart, "N");

        // Upgrade to Team tier so tier-gate passes for all endpoint tests (M16-012)
        await _factory.SeedSubscriptionAsync(_orgGuid, SubscriptionTier.Team);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private object ValidSchedulePayload(string name = "nightly") => new
    {
        name,
        cron = "0 2 * * *",
        collection_ref = "smoke.yaml",
    };

    private string SchedulesUrl => $"/api/v1/organizations/{_orgId}/schedules";

    /// <summary>
    /// Seeds a new user as an org member with the given role by:
    /// 1. Making an authenticated request to trigger user upsert via CurrentUserAccessor.
    /// 2. Directly adding the OrganizationMember row via the factory's DbContext.
    /// </summary>
    private async Task<(HttpClient client, Guid userId)> CreateMemberClientAsync(OrgRole role)
    {
        var userId = Guid.NewGuid();
        var email = $"member-{userId:N}@example.com";
        var token = TestTokens.Create(userId, email);

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        // Make a request to an authenticated endpoint so CurrentUserAccessor upserts the user row
        await memberClient.GetAsync("/api/v1/organizations");

        // Seed the OrganizationMember row directly
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = _orgGuid,
            UserId = userId,
            Role = role,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        return (memberClient, userId);
    }

    // ── Happy path ────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_schedule_returns_201_with_next_run_at_in_future()
    {
        var response = await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("nightly-201"));

        response.StatusCode.Should().Be(HttpStatusCode.Created);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("id").GetString().Should().StartWith("sched_");
        doc.RootElement.GetProperty("name").GetString().Should().Be("nightly-201");
        doc.RootElement.GetProperty("next_run_at").ValueKind.Should().NotBe(JsonValueKind.Null);
        var nextRunAt = DateTime.Parse(doc.RootElement.GetProperty("next_run_at").GetString()!);
        nextRunAt.Should().BeAfter(DateTime.UtcNow.AddMinutes(-1));
    }

    [Fact]
    public async Task Post_schedule_returns_400_invalid_cron_for_bad_expression()
    {
        var body = new { name = "bad-cron", cron = "not-a-cron", collection_ref = "smoke.yaml" };
        var response = await _client.PostAsJsonAsync(SchedulesUrl, body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_cron");
    }

    [Fact]
    public async Task Post_schedule_returns_409_schedule_name_taken_for_duplicate()
    {
        // Create once
        var first = await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("dup-name"));
        first.EnsureSuccessStatusCode();

        // Create again with same name
        var response = await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("dup-name"));

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("schedule_name_taken");
    }

    [Fact]
    public async Task Post_run_now_returns_202_with_queued_run()
    {
        // Create a schedule first
        var created = await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("run-now-test"));
        created.EnsureSuccessStatusCode();

        // Trigger run-now
        var response = await _client.PostAsync($"{SchedulesUrl}/run-now-test/run-now", null);

        response.StatusCode.Should().Be(HttpStatusCode.Accepted);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("run_id").GetString().Should().StartWith("run_");
        doc.RootElement.GetProperty("status").GetString().Should().Be("queued");
    }

    [Fact]
    public async Task Get_schedule_runs_returns_queued_run_after_run_now()
    {
        var name = "get-runs-test";
        await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload(name));
        await _client.PostAsync($"{SchedulesUrl}/{name}/run-now", null);

        var response = await _client.GetAsync($"{SchedulesUrl}/{name}/runs");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var runs = doc.RootElement.GetProperty("runs");
        runs.GetArrayLength().Should().BeGreaterThanOrEqualTo(1);
        runs[0].GetProperty("run_id").GetString().Should().StartWith("run_");
        runs[0].GetProperty("status").GetString().Should().Be("queued");
    }

    [Fact]
    public async Task Get_schedules_returns_200_list_for_org_member()
    {
        // Create a schedule as owner
        await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("list-test"));

        var response = await _client.GetAsync(SchedulesUrl);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var schedules = doc.RootElement.GetProperty("schedules");
        schedules.GetArrayLength().Should().BeGreaterThanOrEqualTo(1,
            because: "the list must include the schedule just created");
    }

    [Fact]
    public async Task Get_schedules_returns_403_for_non_member()
    {
        var (nonMemberClient, _) = (TestTokens.CreateNew($"nonmember-{Guid.NewGuid():N}@example.com"), Guid.Empty);
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", nonMemberClient.token);

        var response = await client.GetAsync(SchedulesUrl);

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    [Fact]
    public async Task Get_schedule_by_name_returns_200_for_org_member()
    {
        var name = "get-by-name-test";
        await _client.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload(name));

        var response = await _client.GetAsync($"{SchedulesUrl}/{name}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("name").GetString().Should().Be(name);
        doc.RootElement.GetProperty("id").GetString().Should().StartWith("sched_");
    }

    [Fact]
    public async Task Get_schedule_by_name_returns_404_for_unknown_name()
    {
        var response = await _client.GetAsync($"{SchedulesUrl}/does-not-exist");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("schedule_not_found");
    }

    // ── RBAC ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_schedule_returns_403_for_member_non_admin()
    {
        // Create a user seeded as Member (not Admin/Owner) in the org
        var (memberClient, _) = await CreateMemberClientAsync(OrgRole.Member);

        var response = await memberClient.PostAsJsonAsync(SchedulesUrl, ValidSchedulePayload("member-rbac-test"));

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    [Fact]
    public async Task All_schedule_endpoints_return_401_without_bearer()
    {
        var anon = _factory.CreateClient();

        var post = await anon.PostAsync(SchedulesUrl,
            new StringContent("{}", Encoding.UTF8, "application/json"));
        var list = await anon.GetAsync(SchedulesUrl);
        var get = await anon.GetAsync($"{SchedulesUrl}/nightly");
        var runNow = await anon.PostAsync($"{SchedulesUrl}/nightly/run-now", null);
        var runs = await anon.GetAsync($"{SchedulesUrl}/nightly/runs");

        post.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        list.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        get.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        runNow.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        runs.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── Swagger ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Swagger_json_lists_schedules_endpoints()
    {
        var response = await _client.GetAsync("/swagger/v1/swagger.json");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        paths.TryGetProperty("/api/v1/organizations/{orgId}/schedules", out var schedulesPath)
            .Should().BeTrue(because: "swagger must include /api/v1/organizations/{orgId}/schedules path");
        schedulesPath.TryGetProperty("post", out var postOp)
            .Should().BeTrue(because: "POST /schedules must be documented");
        schedulesPath.TryGetProperty("get", out _)
            .Should().BeTrue(because: "GET /schedules must be documented");

        // Verify the cron field is documented in the POST request body schema (DoD requirement).
        // The schema may be inlined or referenced via $ref to components/schemas.
        postOp.TryGetProperty("requestBody", out var requestBody)
            .Should().BeTrue(because: "POST /schedules must have a documented request body");

        var schemaElement = requestBody
            .GetProperty("content")
            .GetProperty("application/json")
            .GetProperty("schema");

        bool cronFound;
        bool timezoneFound;
        if (schemaElement.TryGetProperty("properties", out var inlineProps))
        {
            // Inline schema
            cronFound = inlineProps.TryGetProperty("cron", out _)
                        || inlineProps.TryGetProperty("Cron", out _);
            timezoneFound = inlineProps.TryGetProperty("timezone", out _)
                            || inlineProps.TryGetProperty("Timezone", out _);
        }
        else if (schemaElement.TryGetProperty("$ref", out var refElement))
        {
            // Resolve $ref — e.g. "#/components/schemas/CreateScheduleRequest"
            var refPath = refElement.GetString()!;
            var typeName = refPath.Split('/').Last();
            var schemaProps = doc.RootElement
                .GetProperty("components")
                .GetProperty("schemas")
                .GetProperty(typeName)
                .GetProperty("properties");
            cronFound = schemaProps.TryGetProperty("cron", out _)
                        || schemaProps.TryGetProperty("Cron", out _);
            timezoneFound = schemaProps.TryGetProperty("timezone", out _)
                            || schemaProps.TryGetProperty("Timezone", out _);
        }
        else
        {
            cronFound = false;
            timezoneFound = false;
        }

        cronFound.Should().BeTrue(because: "the POST /schedules request body schema must document the cron field");
        timezoneFound.Should().BeTrue(because: "the POST /schedules request body schema must document the timezone field (DoD requirement)");

        // Verify the 422 response is documented (DoD: OpenAPI doc updated for timezone validation)
        postOp.GetProperty("responses").TryGetProperty("422", out _)
            .Should().BeTrue(because: "POST /schedules must document the 422 invalid-timezone response");
    }

    // ── Timezone (M16-012) ────────────────────────────────────────────────────

    [Fact]
    public async Task Post_schedule_returns_422_invalid_timezone_for_bad_tz()
    {
        var body = new { name = "bad-tz-test", cron = "0 9 * * *", collection_ref = "smoke.yaml", timezone = "Atlantis/Lost" };
        var response = await _client.PostAsJsonAsync(SchedulesUrl, body);

        response.StatusCode.Should().Be(HttpStatusCode.UnprocessableEntity);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("type").GetString()
            .Should().EndWith("/invalid-timezone",
                because: "type must match the RFC 7807 invalid-timezone format");
        doc.RootElement.TryGetProperty("valid_examples", out var examples)
            .Should().BeTrue(because: "response must include valid_examples extension");
        examples.EnumerateArray().Select(e => e.GetString()).Should().Contain("UTC");
    }

    [Fact]
    public async Task Post_schedule_returns_201_with_timezone_in_response()
    {
        var body = new { name = "tz-response-test", cron = "0 9 * * *", collection_ref = "smoke.yaml", timezone = "Asia/Tokyo" };
        var response = await _client.PostAsJsonAsync(SchedulesUrl, body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("timezone").GetString().Should().Be("Asia/Tokyo");
    }

    [Fact]
    public async Task List_schedules_returns_402_for_free_tier_org()
    {
        // Create a fresh user and org with no subscription (Free tier)
        var userId = Guid.NewGuid();
        var email = $"free-sched-{userId:N}@example.com";
        var token = TestTokens.Create(userId, email);
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        var slug = $"free-{userId:N}"[..20];
        var resp = await freeClient.PostAsJsonAsync("/api/v1/organizations", new { name = "FreeOrg", slug });
        resp.EnsureSuccessStatusCode();
        var orgId = JsonDocument.Parse(await resp.Content.ReadAsStringAsync())
            .RootElement.GetProperty("id").GetString()!;

        var listResp = await freeClient.GetAsync($"/api/v1/organizations/{orgId}/schedules");
        listResp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired,
            because: "free-tier orgs must get 402 on schedule endpoints");
        var body = await listResp.Content.ReadAsStringAsync();
        body.Should().Contain("schedule_executor_tier_ineligible");
    }

    /// <summary>
    /// Creates a fresh free-tier org client (no subscription seeded) for tier-gate tests.
    /// </summary>
    private async Task<(HttpClient client, string orgId)> CreateFreeTierClientAsync()
    {
        var userId = Guid.NewGuid();
        var email = $"free-gate-{userId:N}@example.com";
        var token = TestTokens.Create(userId, email);
        var freeClient = _factory.CreateClient();
        freeClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);

        var slug = $"fg-{userId:N}"[..20];
        var resp = await freeClient.PostAsJsonAsync("/api/v1/organizations", new { name = "FreeGateOrg", slug });
        resp.EnsureSuccessStatusCode();
        var orgId = JsonDocument.Parse(await resp.Content.ReadAsStringAsync())
            .RootElement.GetProperty("id").GetString()!;
        return (freeClient, orgId);
    }

    [Fact]
    public async Task Post_schedule_returns_402_for_free_tier_org()
    {
        var (freeClient, orgId) = await CreateFreeTierClientAsync();
        var resp = await freeClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/schedules",
            ValidSchedulePayload("gate-create"));
        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired,
            because: "free-tier orgs must get 402 on create schedule");
        (await resp.Content.ReadAsStringAsync()).Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task Get_schedule_returns_402_for_free_tier_org()
    {
        var (freeClient, orgId) = await CreateFreeTierClientAsync();
        var resp = await freeClient.GetAsync($"/api/v1/organizations/{orgId}/schedules/any-name");
        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired,
            because: "free-tier orgs must get 402 on get schedule");
        (await resp.Content.ReadAsStringAsync()).Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task RunNow_returns_402_for_free_tier_org()
    {
        var (freeClient, orgId) = await CreateFreeTierClientAsync();
        var resp = await freeClient.PostAsync($"/api/v1/organizations/{orgId}/schedules/any-name/run-now", null);
        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired,
            because: "free-tier orgs must get 402 on run-now");
        (await resp.Content.ReadAsStringAsync()).Should().Contain("schedule_executor_tier_ineligible");
    }

    [Fact]
    public async Task ListRuns_returns_402_for_free_tier_org()
    {
        var (freeClient, orgId) = await CreateFreeTierClientAsync();
        var resp = await freeClient.GetAsync($"/api/v1/organizations/{orgId}/schedules/any-name/runs");
        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired,
            because: "free-tier orgs must get 402 on list runs");
        (await resp.Content.ReadAsStringAsync()).Should().Contain("schedule_executor_tier_ineligible");
    }
}
