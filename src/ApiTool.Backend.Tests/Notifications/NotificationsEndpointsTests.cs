using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Notifications;

/// <summary>HTTP integration tests for the notifications endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class NotificationsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public NotificationsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        // Create an org for this test instance
        var slug = $"notif-{_ownerId:N}"[..20];
        var body = new { name = "NotifTestOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── POST /notification-rules ──────────────────────────────────────────────

    [Fact]
    public async Task Post_notification_rules_returns_201_with_slack_rule()
    {
        var body = new { channel = "slack", target = "https://hooks.slack.test/xyz", on = new[] { "run_failed" } };
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("id").GetString().Should().StartWith("nrule_");
        doc.RootElement.GetProperty("channel").GetString().Should().Be("slack");
        doc.RootElement.GetProperty("target").GetString().Should().Be("https://hooks.slack.test/xyz");
        var onArr = doc.RootElement.GetProperty("on");
        onArr.EnumerateArray().Select(e => e.GetString()).Should().Contain("run_failed");
    }

    [Fact]
    public async Task Post_notification_rules_returns_201_with_email_rule_multiple_events()
    {
        var body = new { channel = "email", target = "alerts@acme.com", on = new[] { "run_failed", "flaky" } };
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("channel").GetString().Should().Be("email");
        var onArr = doc.RootElement.GetProperty("on").EnumerateArray()
            .Select(e => e.GetString())
            .ToList();
        onArr.Should().Contain("run_failed");
        onArr.Should().Contain("flaky");
    }

    [Fact]
    public async Task Post_notification_rules_returns_400_invalid_channel()
    {
        var body = new { channel = "telegram", target = "some-bot", on = new[] { "run_failed" } };
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_channel");
    }

    [Fact]
    public async Task Post_notification_rules_returns_403_for_non_admin_member()
    {
        // Create a member user and directly seed an organization_members row with Member role,
        // so the test truly exercises "authenticated member (not admin) returns 403" rather
        // than "non-member returns 403".
        var memberId = Guid.NewGuid();
        var memberToken = TestTokens.Create(memberId, $"member-{memberId:N}@example.com");

        // Parse wire-format orgId back to Guid for DB seeding
        ApiTool.Backend.Organizations.OrgId.TryParse(_orgId, out var orgGuid);

        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Users.Add(new User
            {
                Id = memberId,
                Email = $"member-{memberId:N}@example.com",
                CreatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgGuid,
                UserId = memberId,
                Role = OrgRole.Member,
                JoinedAt = DateTime.UtcNow,
                InvitedBy = _ownerId,
            });
            await db.SaveChangesAsync();
        }

        // Use member client — must get 403 because Member role cannot create rules
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var body = new { channel = "slack", target = "https://hooks.slack.test/xyz", on = new[] { "run_failed" } };
        var response = await memberClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── GET /notification-deliveries ─────────────────────────────────────────

    [Fact]
    public async Task Get_notification_deliveries_returns_newest_first()
    {
        // First create a rule
        var ruleBody = new { channel = "slack", target = "https://hooks.slack.test/del", on = new[] { "run_failed" } };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/notification-rules", ruleBody);

        // Ingest a failing run to trigger a delivery
        var resultBody = new
        {
            collection_name = "delivery-test",
            run_at = "2026-04-17T12:00:00Z",
            duration_ms = 500,
            pass_count = 1,
            fail_count = 1,
            skipped_count = 0,
            triggered_by = "cli",
            git_sha = "abc123",
            items = new object[]
            {
                new { name = "test-1", status = "passed", duration_ms = 100, message = (string?)null },
                new { name = "test-2", status = "failed", duration_ms = 400, message = "Expected 204" },
            },
        };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/results", resultBody);

        var response = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-deliveries");

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var deliveries = doc.RootElement.GetProperty("deliveries").EnumerateArray().ToList();
        deliveries.Should().NotBeEmpty();

        var first = deliveries[0];
        first.GetProperty("id").GetString().Should().StartWith("ndel_");
        first.GetProperty("rule_id").GetString().Should().StartWith("nrule_");
        first.GetProperty("channel").GetString().Should().Be("slack");
        first.GetProperty("status").GetString().Should().BeOneOf("delivered", "failed");
    }

    // ── Integration: ingest triggers dispatch ─────────────────────────────────

    [Fact]
    public async Task Ingesting_failing_run_triggers_slack_post_and_delivery_row()
    {
        // Create a slack rule
        var ruleBody = new { channel = "slack", target = "https://hooks.slack.test/fire", on = new[] { "run_failed" } };
        var ruleResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/notification-rules", ruleBody);
        ruleResp.StatusCode.Should().Be(HttpStatusCode.Created);

        // Ingest a failing result
        var resultBody = new
        {
            collection_name = "integration-fire",
            run_at = "2026-04-17T12:00:00Z",
            duration_ms = 800,
            pass_count = 0,
            fail_count = 2,
            skipped_count = 0,
            triggered_by = "cli",
            git_sha = "fire123",
            items = new[]
            {
                new { name = "test-a", status = "failed", duration_ms = 400, message = "oops" },
                new { name = "test-b", status = "failed", duration_ms = 400, message = "again" },
            },
        };
        // Snapshot call count before ingest so we can detect a new call even if
        // earlier tests in the same collection already accumulated entries.
        var fakeSlack = _factory.GetFakeSlackPoster();
        fakeSlack.Should().NotBeNull(because: "BackendFactory should expose FakeSlackWebhookPoster");
        var callsBefore = fakeSlack!.Calls.Count;

        var ingestResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/results", resultBody);
        ingestResp.StatusCode.Should().Be(HttpStatusCode.Accepted);

        // List deliveries — should have one
        var delivResp = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-deliveries");
        delivResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await delivResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var deliveries = doc.RootElement.GetProperty("deliveries").EnumerateArray().ToList();
        deliveries.Should().NotBeEmpty(because: "ingest should have triggered dispatch");

        fakeSlack.Calls.Count.Should().BeGreaterThan(callsBefore,
            because: "dispatcher should have posted to Slack for this test's ingest");
    }

    [Fact]
    public async Task Ingesting_passing_run_records_no_delivery()
    {
        // Create a rule
        var ruleBody = new { channel = "slack", target = "https://hooks.slack.test/nofire", on = new[] { "run_failed" } };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/notification-rules", ruleBody);

        // Ingest a passing result
        var resultBody = new
        {
            collection_name = "passing-test",
            run_at = "2026-04-17T12:00:00Z",
            duration_ms = 500,
            pass_count = 3,
            fail_count = 0,
            skipped_count = 0,
            triggered_by = "cli",
            git_sha = "pass123",
            items = new[]
            {
                new { name = "t1", status = "passed", duration_ms = 100, message = (string?)null },
                new { name = "t2", status = "passed", duration_ms = 200, message = (string?)null },
                new { name = "t3", status = "passed", duration_ms = 200, message = (string?)null },
            },
        };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/results", resultBody);

        var delivResp = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-deliveries");
        var json = await delivResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var deliveries = doc.RootElement.GetProperty("deliveries").EnumerateArray().ToList();
        deliveries.Should().BeEmpty(because: "no delivery should fire for a passing run");
    }

    // ── GET /notification-rules ───────────────────────────────────────────────

    [Fact]
    public async Task Get_notification_rules_returns_200_with_rules_oldest_first()
    {
        // Seed two rules
        var body1 = new { channel = "slack", target = "https://hooks.slack.test/a", on = new[] { "run_failed" } };
        var body2 = new { channel = "email", target = "alerts@acme.com", on = new[] { "flaky" } };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/notification-rules", body1);
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/notification-rules", body2);

        var response = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-rules");

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var rules = doc.RootElement.GetProperty("rules").EnumerateArray().ToList();
        rules.Should().HaveCount(2);
        rules[0].GetProperty("id").GetString().Should().StartWith("nrule_");
        rules[0].GetProperty("channel").GetString().Should().Be("slack");
        rules[1].GetProperty("channel").GetString().Should().Be("email");
    }

    [Fact]
    public async Task Get_notification_rules_returns_empty_list_when_no_rules()
    {
        var response = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-rules");

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var rules = doc.RootElement.GetProperty("rules").EnumerateArray().ToList();
        rules.Should().BeEmpty();
    }

    [Fact]
    public async Task Get_notification_rules_returns_403_for_non_member()
    {
        // A user with no org membership
        var nonMemberId = Guid.NewGuid();
        var nonMemberToken = TestTokens.Create(nonMemberId, $"non-{nonMemberId:N}@example.com");
        var nonMemberClient = _factory.CreateClient();
        nonMemberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", nonMemberToken);

        var response = await nonMemberClient.GetAsync($"/api/v1/organizations/{_orgId}/notification-rules");

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── DELETE /notification-rules/{ruleId} ───────────────────────────────────

    [Fact]
    public async Task Delete_notification_rule_returns_204_for_admin()
    {
        // Create a rule then delete it
        var body = new { channel = "slack", target = "https://hooks.slack.test/del204", on = new[] { "run_failed" } };
        var createResp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);

        var created = await createResp.Content.ReadAsStringAsync();
        using var createdDoc = JsonDocument.Parse(created);
        var ruleId = createdDoc.RootElement.GetProperty("id").GetString()!;

        var deleteResp = await _client.DeleteAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules/{ruleId}");
        deleteResp.StatusCode.Should().Be(HttpStatusCode.NoContent);

        // Verify it's gone
        var listResp = await _client.GetAsync($"/api/v1/organizations/{_orgId}/notification-rules");
        var listJson = await listResp.Content.ReadAsStringAsync();
        using var listDoc = JsonDocument.Parse(listJson);
        listDoc.RootElement.GetProperty("rules").EnumerateArray().ToList().Should().BeEmpty();
    }

    [Fact]
    public async Task Delete_notification_rule_returns_404_for_unknown_id()
    {
        // Valid format id that does not exist
        var fakeId = $"nrule_{Guid.NewGuid():N}";
        var response = await _client.DeleteAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules/{fakeId}");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("not_found");
    }

    [Fact]
    public async Task Delete_notification_rule_returns_403_for_member_role()
    {
        // Create rule as owner
        var body = new { channel = "slack", target = "https://hooks.slack.test/del403", on = new[] { "run_failed" } };
        var createResp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules", body);
        var created = await createResp.Content.ReadAsStringAsync();
        using var createdDoc = JsonDocument.Parse(created);
        var ruleId = createdDoc.RootElement.GetProperty("id").GetString()!;

        // Seed a member
        var memberId = Guid.NewGuid();
        var memberToken = TestTokens.Create(memberId, $"del-member-{memberId:N}@example.com");
        ApiTool.Backend.Organizations.OrgId.TryParse(_orgId, out var orgGuid);
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.Users.Add(new User { Id = memberId, Email = $"del-member-{memberId:N}@example.com", CreatedAt = DateTime.UtcNow });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = orgGuid,
                UserId = memberId,
                Role = OrgRole.Member,
                JoinedAt = DateTime.UtcNow,
                InvitedBy = _ownerId,
            });
            await db.SaveChangesAsync();
        }

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var deleteResp = await memberClient.DeleteAsync(
            $"/api/v1/organizations/{_orgId}/notification-rules/{ruleId}");

        deleteResp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── Swagger ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Swagger_json_lists_notification_endpoints()
    {
        var response = await _client.GetAsync("/swagger/v1/swagger.json");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("notification-rules");
        json.Should().Contain("notification-deliveries");
    }
}
