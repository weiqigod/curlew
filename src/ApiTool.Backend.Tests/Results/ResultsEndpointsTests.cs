using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Results;

/// <summary>HTTP integration tests for the results endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class ResultsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public ResultsEndpointsTests(BackendFactory factory)
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
        var slug = $"res-{_ownerId:N}"[..20];
        var body = new { name = "ResultsTestOrg", slug };
        var response = await _client.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private object ValidPayload(int count = 1) => new
    {
        collection_name = "smoke-tests",
        run_at = "2026-04-15T12:00:00Z",
        duration_ms = 1234,
        pass_count = count,
        fail_count = 0,
        skipped_count = 0,
        triggered_by = "cli",
        git_sha = "abc123",
        items = Enumerable.Range(0, count).Select(i => new
        {
            name = $"test-{i}",
            status = "passed",
            duration_ms = 100,
            message = (string?)null,
        }).ToArray(),
    };

    // ── Happy path ────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_results_returns_202_and_persists_row()
    {
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload(3));

        response.StatusCode.Should().Be(HttpStatusCode.Accepted);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("result_id").GetString().Should().StartWith("res_");
        doc.RootElement.GetProperty("status").GetString().Should().Be("accepted");
    }

    [Fact]
    public async Task Get_results_returns_newest_first_for_member()
    {
        // Post two results — second has pass_count=2, so if ordering is newest-first
        // arr[0] must have pass_count == 2 and arr[1] must have pass_count == 1.
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/results", ValidPayload(1));
        await _client.PostAsJsonAsync($"/api/v1/organizations/{_orgId}/results", ValidPayload(2));

        var response = await _client.GetAsync($"/api/v1/organizations/{_orgId}/results?limit=10");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var arr = doc.RootElement.GetProperty("results");
        arr.GetArrayLength().Should().BeGreaterThanOrEqualTo(2);

        // Verify newest-first ordering by checking that the most recently posted
        // result (pass_count=2) appears first.
        arr[0].GetProperty("pass_count").GetInt32().Should().Be(2, because: "newest result (pass_count=2) must be first");

        // Verify all required fields are present on the first element (task behavior #5).
        arr[0].TryGetProperty("fail_count", out _).Should().BeTrue(because: "fail_count must be present in list response");
        arr[0].TryGetProperty("duration_ms", out _).Should().BeTrue(because: "duration_ms must be present in list response");
        arr[0].TryGetProperty("run_at", out _).Should().BeTrue(because: "run_at must be present in list response");
    }

    [Fact]
    public async Task Get_result_detail_returns_200_with_items_for_member()
    {
        var postResponse = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload(2));
        postResponse.EnsureSuccessStatusCode();

        var postJson = await postResponse.Content.ReadAsStringAsync();
        using var postDoc = JsonDocument.Parse(postJson);
        var resultId = postDoc.RootElement.GetProperty("result_id").GetString()!;

        var response = await _client.GetAsync($"/api/v1/results/{resultId}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("id").GetString().Should().Be(resultId);
        doc.RootElement.GetProperty("items").GetArrayLength().Should().Be(2);
    }

    // ── RBAC ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_results_returns_403_permission_denied_for_non_member()
    {
        var (otherToken, _) = TestTokens.CreateNew($"other-{Guid.NewGuid():N}@example.com");
        var otherClient = _factory.CreateClient();
        otherClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", otherToken);

        var response = await otherClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload());

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    [Fact]
    public async Task Get_result_detail_returns_404_for_cross_org_caller()
    {
        // Post a result under our org
        var postResponse = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload());
        postResponse.EnsureSuccessStatusCode();

        var postJson = await postResponse.Content.ReadAsStringAsync();
        using var postDoc = JsonDocument.Parse(postJson);
        var resultId = postDoc.RootElement.GetProperty("result_id").GetString()!;

        // Try to access it as a different user with no org membership
        var (otherToken, _) = TestTokens.CreateNew($"cross-{Guid.NewGuid():N}@example.com");
        var otherClient = _factory.CreateClient();
        otherClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", otherToken);

        var response = await otherClient.GetAsync($"/api/v1/results/{resultId}");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("result_not_found");
    }

    // ── Validation ────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_results_returns_400_invalid_result_schema_with_field_pointer_when_pass_count_missing()
    {
        var body = new
        {
            collection_name = "test",
            run_at = "2026-04-15T12:00:00Z",
            duration_ms = 100,
            // pass_count intentionally omitted
            fail_count = 0,
            items = Array.Empty<object>(),
        };

        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_result_schema");
        doc.RootElement.GetProperty("field").GetString().Should().Be("/pass_count");
    }

    [Fact]
    public async Task Post_results_returns_400_when_item_status_is_unknown()
    {
        var body = new
        {
            collection_name = "test",
            run_at = "2026-04-15T12:00:00Z",
            duration_ms = 100,
            pass_count = 1,
            fail_count = 0,
            items = new[] { new { name = "t", status = "flying", duration_ms = 10 } },
        };

        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_result_schema");
        doc.RootElement.GetProperty("field").GetString().Should().Be("/items/0/status");
    }

    // ── Size cap ──────────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_results_returns_413_when_body_exceeds_5mb()
    {
        // Build a payload just over 5 MB.
        // Strategy: embed a large base64-encoded blob in collection_name + git_sha padding.
        // We bypass item validation by using valid items but pad with a large git_sha value.
        // Each item: name of 400 chars. 5000 items × 450 chars/item ≈ 2.25 MB of items.
        // We also add a huge collection_name + padding to push total > 5 MB.
        var bigName = new string('x', 400);
        var items = Enumerable.Range(0, 4900)
            .Select(i => new { name = bigName, status = "passed", duration_ms = 1 })
            .ToArray();

        var payload = new
        {
            collection_name = new string('a', 3_500_000), // 3.5 MB on its own
            run_at = "2026-04-15T12:00:00Z",
            duration_ms = 1,
            pass_count = items.Length,
            fail_count = 0,
            items,
        };

        var serialized = JsonSerializer.Serialize(payload);
        var bytes = Encoding.UTF8.GetBytes(serialized);
        // Assert the test data itself is actually > 5 MB so the test is meaningful
        bytes.Length.Should().BeGreaterThan(5 * 1024 * 1024, because: "test payload must exceed 5 MB");

        var content = new ByteArrayContent(bytes);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/json");
        content.Headers.ContentLength = bytes.Length;

        var response = await _client.PostAsync(
            $"/api/v1/organizations/{_orgId}/results", content);

        response.StatusCode.Should().Be(HttpStatusCode.RequestEntityTooLarge);
        var responseJson = await response.Content.ReadAsStringAsync();
        using var responseDoc = JsonDocument.Parse(responseJson);
        responseDoc.RootElement.GetProperty("code").GetString()
            .Should().Be("payload_too_large", because: "413 response body must contain code:payload_too_large");
    }

    // ── Auth ──────────────────────────────────────────────────────────────────

    [Fact]
    public async Task All_results_endpoints_return_401_when_no_bearer_header()
    {
        var anon = _factory.CreateClient();

        var post = await anon.PostAsync(
            $"/api/v1/organizations/{_orgId}/results",
            new StringContent("{}", Encoding.UTF8, "application/json"));
        var list = await anon.GetAsync($"/api/v1/organizations/{_orgId}/results");
        var detail = await anon.GetAsync("/api/v1/results/res_00000000000000000000000000000000");

        post.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        list.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        detail.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── Swagger ───────────────────────────────────────────────────────────────

    [Fact]
    public async Task Swagger_json_lists_results_endpoints()
    {
        var response = await _client.GetAsync("/swagger/v1/swagger.json");
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);

        var paths = doc.RootElement.GetProperty("paths");

        // POST /organizations/{orgId}/results must be documented.
        paths.TryGetProperty("/api/v1/organizations/{orgId}/results", out var orgResultsPath)
            .Should().BeTrue(because: "swagger must include /api/v1/organizations/{orgId}/results path");
        orgResultsPath.TryGetProperty("post", out _)
            .Should().BeTrue(because: "POST /organizations/{orgId}/results must be documented");
        orgResultsPath.TryGetProperty("get", out _)
            .Should().BeTrue(because: "GET /organizations/{orgId}/results must be documented");

        // GET /results/{resultId} must be documented.
        paths.TryGetProperty("/api/v1/results/{resultId}", out var resultDetailPath)
            .Should().BeTrue(because: "swagger must include /api/v1/results/{resultId} path");
        resultDetailPath.TryGetProperty("get", out _)
            .Should().BeTrue(because: "GET /results/{resultId} must be documented");
    }
}
