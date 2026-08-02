using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Coordinator;

/// <summary>HTTP integration tests for the coordinator endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class CoordinatorEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public CoordinatorEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"coord-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"coord-{_ownerId:N}"[..20];
        var response = await _client.PostAsJsonAsync("/api/v1/organizations",
            new { name = "CoordTestOrg", slug });
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private string JobsUrl => $"/api/v1/organizations/{_orgId}/coordinator/jobs";

    private static async Task<string?> GetStringProp(HttpResponseMessage resp, string prop)
    {
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.TryGetProperty(prop, out var el) ? el.GetString() : null;
    }

    // ── 1. Create job ─────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_jobs_returns_201_with_shards_pending()
    {
        var response = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "abc123", shard_count = 4 });

        response.StatusCode.Should().Be(HttpStatusCode.Created);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("job_id").GetString().Should().StartWith("job_");
        var shards = doc.RootElement.GetProperty("shards").EnumerateArray().ToList();
        shards.Should().HaveCount(4);
        shards.Should().AllSatisfy(s => s.GetProperty("state").GetString().Should().Be("pending"));
        shards.Should().AllSatisfy(s => s.GetProperty("shard_id").GetString().Should().StartWith("shd_"));
    }

    // ── 2. Claim shard ────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_claim_returns_200_with_running_shard()
    {
        // Create job
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "claim-test", shard_count = 2 });
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);
        var jobId = await GetStringProp(createResp, "job_id");

        // Claim
        var claimResp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1", capabilities = new[] { "http" } });

        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await claimResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("state").GetString().Should().Be("running");
        doc.RootElement.GetProperty("assigned_worker").GetString().Should().Be("w1");
    }

    // ── 3. All shards claimed → 204 ───────────────────────────────────────────

    [Fact]
    public async Task Post_claim_returns_204_when_all_shards_claimed()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "all-claimed", shard_count = 2 });
        var jobId = await GetStringProp(createResp, "job_id");

        await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim", new { worker_id = "w1" });
        await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim", new { worker_id = "w2" });

        var resp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim", new { worker_id = "w3" });
        resp.StatusCode.Should().Be(HttpStatusCode.NoContent);
    }

    // ── 4. Submit result (last shard triggers aggregate) ─────────────────────

    [Fact]
    public async Task Post_result_transitions_shard_completed_and_appends_aggregate()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "aggregate-sha", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        claimResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        var resultResp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w1", pass_count = 2, fail_count = 0, duration_ms = 300, items = Array.Empty<object>() });

        resultResp.StatusCode.Should().Be(HttpStatusCode.Accepted);

        // Verify the job is now completed
        var jobResp = await _client.GetAsync($"{JobsUrl}/{jobId}");
        jobResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var jobBody = await jobResp.Content.ReadAsStringAsync();
        using var jobDoc = JsonDocument.Parse(jobBody);
        jobDoc.RootElement.GetProperty("state").GetString().Should().Be("completed");
        jobDoc.RootElement.GetProperty("aggregate_result_id").GetString().Should().StartWith("res_");
    }

    // ── 5. Non-member → 403 ───────────────────────────────────────────────────

    [Fact]
    public async Task Post_jobs_returns_403_for_non_member()
    {
        var (nonMemberToken, _) = TestTokens.CreateNew("nonmember@example.com");
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", nonMemberToken);

        var resp = await client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "x", shard_count = 1 });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 6. No bearer → 401 ───────────────────────────────────────────────────

    [Fact]
    public async Task All_coordinator_endpoints_return_401_without_bearer()
    {
        var anonClient = _factory.CreateClient();

        var endpoints = new[]
        {
            (HttpMethod.Post, JobsUrl),
            (HttpMethod.Get,  $"{JobsUrl}/job_fake"),
            (HttpMethod.Post, $"{JobsUrl}/job_fake/claim"),
            (HttpMethod.Post, $"{JobsUrl}/job_fake/shards/shd_fake/result"),
            (HttpMethod.Post, $"{JobsUrl}/job_fake/shards/shd_fake/heartbeat"),
        };

        foreach (var (method, url) in endpoints)
        {
            using var req = new HttpRequestMessage(method, url)
            {
                Content = JsonContent.Create(new { }),
            };
            var resp = await anonClient.SendAsync(req);
            resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized,
                because: $"{method} {url} should require authentication");
        }
    }

    // ── 7. Get job ────────────────────────────────────────────────────────────

    [Fact]
    public async Task Get_job_returns_current_state_with_shard_counts()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "get-test", shard_count = 3 });
        var jobId = await GetStringProp(createResp, "job_id");

        var resp = await _client.GetAsync($"{JobsUrl}/{jobId}");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("job_id").GetString().Should().Be(jobId);
        doc.RootElement.GetProperty("shard_count").GetInt32().Should().Be(3);
        doc.RootElement.GetProperty("shards").GetArrayLength().Should().Be(3);
    }

    // ── 8. Heartbeat ─────────────────────────────────────────────────────────

    [Fact]
    public async Task Post_heartbeat_returns_204()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-test", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w-hb" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        var hbResp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/heartbeat",
            new { worker_id = "w-hb" });

        hbResp.StatusCode.Should().Be(HttpStatusCode.NoContent);
    }

    // ── 9. Wrong worker on result → 403 ──────────────────────────────────────

    [Fact]
    public async Task Post_result_rejects_wrong_worker_with_403()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "wrong-worker", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        // w2 tries to submit for w1's shard
        var resultResp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w2", pass_count = 1, fail_count = 0, duration_ms = 100 });

        resultResp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 10. Swagger lists coordinator endpoints ────────────────────────────────

    [Fact]
    public async Task Swagger_json_lists_coordinator_endpoints()
    {
        var resp = await _client.GetAsync("/swagger/v1/swagger.json");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("coordinator/jobs",
            because: "Swagger should list the coordinator jobs endpoint");
        body.Should().Contain("coordinator/jobs/{jobId}/claim",
            because: "Swagger should list the coordinator claim endpoint");
    }

    // ── 11. POST result on already-completed shard → 400 ─────────────────────

    [Fact]
    public async Task Post_result_on_completed_shard_returns_400()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "completed-shard-test", shard_count = 2 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        // Submit the shard once (shard now Completed, but job is not done — 2 shards total)
        await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w1", pass_count = 1, fail_count = 0, duration_ms = 100, items = Array.Empty<object>() });

        // Submit a second time — shard is already Completed
        var resp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w1", pass_count = 1, fail_count = 0, duration_ms = 100, items = Array.Empty<object>() });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 12. POST heartbeat with wrong worker → 403 ────────────────────────────

    [Fact]
    public async Task Post_heartbeat_returns_403_for_wrong_worker()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-wrong-worker", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        // w2 sends heartbeat for w1's shard
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/heartbeat",
            new { worker_id = "w2" });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 13. POST heartbeat for non-running shard → 400 ───────────────────────

    [Fact]
    public async Task Post_heartbeat_on_completed_shard_returns_400()
    {
        // Use a 2-shard job so completing one shard doesn't complete the job.
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-completed-shard", shard_count = 2 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        // Complete the shard
        await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w1", pass_count = 1, fail_count = 0, duration_ms = 100, items = Array.Empty<object>() });

        // Heartbeat on a completed shard → 400
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 14. GET job with unknown jobId → 404 ─────────────────────────────────

    [Fact]
    public async Task Get_job_returns_404_for_unknown_job_id()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.GetAsync($"{JobsUrl}/{fakeJobId}");
        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 15. POST result with malformed body → 400 ─────────────────────────────

    [Fact]
    public async Task Post_result_with_malformed_json_returns_400()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "malformed-body", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        using var badContent = new StringContent("{not valid json", System.Text.Encoding.UTF8, "application/json");
        var resp = await _client.PostAsync($"{JobsUrl}/{jobId}/shards/{shardId}/result", badContent);

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 16. POST jobs with malformed JSON body → 400 ─────────────────────────

    [Fact]
    public async Task Post_jobs_with_malformed_json_returns_400()
    {
        using var badContent = new StringContent("{not valid json", System.Text.Encoding.UTF8, "application/json");
        var resp = await _client.PostAsync(JobsUrl, badContent);

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 17. POST jobs with invalid orgId in URL → 403 ────────────────────────

    [Fact]
    public async Task Post_jobs_with_invalid_org_id_returns_403()
    {
        var resp = await _client.PostAsJsonAsync(
            "/api/v1/organizations/not-an-org-id/coordinator/jobs",
            new { collection_sha = "x", shard_count = 1 });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 18. POST jobs with invalid shard_count → 400 ─────────────────────────

    [Fact]
    public async Task Post_jobs_with_zero_shard_count_returns_400_invalid_shard_count()
    {
        var resp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "valid-sha", shard_count = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_shard_count");
    }

    // ── 19. POST jobs with missing collection_sha → 400 invalid_request ───────

    [Fact]
    public async Task Post_jobs_with_missing_collection_sha_returns_400_invalid_request()
    {
        var resp = await _client.PostAsJsonAsync(JobsUrl,
            new { shard_count = 2 });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_request");
    }

    // ── 20. POST claim with malformed JSON body → 400 ────────────────────────

    [Fact]
    public async Task Post_claim_with_malformed_json_returns_400()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "claim-malformed", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        using var badContent = new StringContent("{not valid json", System.Text.Encoding.UTF8, "application/json");
        var resp = await _client.PostAsync($"{JobsUrl}/{jobId}/claim", badContent);

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 21. POST claim with invalid orgId → 403 ──────────────────────────────

    [Fact]
    public async Task Post_claim_with_invalid_org_id_returns_403()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/not-an-org-id/coordinator/jobs/{fakeJobId}/claim",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 22. POST claim with unparseable jobId → 404 ──────────────────────────

    [Fact]
    public async Task Post_claim_with_unparseable_job_id_returns_404()
    {
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/not-a-job-id/claim",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 23. POST claim with unknown jobId → 404 ──────────────────────────────

    [Fact]
    public async Task Post_claim_with_unknown_job_id_returns_404()
    {
        var unknownJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{unknownJobId}/claim",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 24. POST claim with missing worker_id → 400 ──────────────────────────

    [Fact]
    public async Task Post_claim_with_missing_worker_id_returns_400()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "missing-worker-id", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var resp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 25. GET job with invalid orgId → 403 ─────────────────────────────────

    [Fact]
    public async Task Get_job_with_invalid_org_id_returns_403()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.GetAsync(
            $"/api/v1/organizations/not-an-org-id/coordinator/jobs/{fakeJobId}");

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 26. GET job as non-member → 403 ──────────────────────────────────────

    [Fact]
    public async Task Get_job_returns_403_for_non_member()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "get-403-test", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var (nonMemberToken, _) = TestTokens.CreateNew("nonmember-get@example.com");
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", nonMemberToken);

        var resp = await client.GetAsync($"{JobsUrl}/{jobId}");
        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 27. POST result with invalid orgId → 403 ─────────────────────────────

    [Fact]
    public async Task Post_result_with_invalid_org_id_returns_403()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var fakeShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/not-an-org-id/coordinator/jobs/{fakeJobId}/shards/{fakeShardId}/result",
            new { worker_id = "w1", pass_count = 0, fail_count = 0, duration_ms = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 28. POST result with unparseable jobId → 404 ─────────────────────────

    [Fact]
    public async Task Post_result_with_unparseable_job_id_returns_404()
    {
        var fakeShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/not-a-job-id/shards/{fakeShardId}/result",
            new { worker_id = "w1", pass_count = 0, fail_count = 0, duration_ms = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 29. POST result with unparseable shardId → 404 ───────────────────────

    [Fact]
    public async Task Post_result_with_unparseable_shard_id_returns_404()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{fakeJobId}/shards/not-a-shard-id/result",
            new { worker_id = "w1", pass_count = 0, fail_count = 0, duration_ms = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 30. POST result as non-member → 403 ──────────────────────────────────

    [Fact]
    public async Task Post_result_returns_403_for_non_member()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "result-403-test", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        var (nonMemberToken, _) = TestTokens.CreateNew("nonmember-result@example.com");
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", nonMemberToken);

        var resp = await client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/result",
            new { worker_id = "w1", pass_count = 0, fail_count = 0, duration_ms = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 31. POST result with unknown shardId → 404 ───────────────────────────

    [Fact]
    public async Task Post_result_with_unknown_shard_id_returns_404()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "unknown-shard-result", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var unknownShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{unknownShardId}/result",
            new { worker_id = "w1", pass_count = 0, fail_count = 0, duration_ms = 0 });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 32. POST heartbeat with malformed JSON body → 400 ────────────────────

    [Fact]
    public async Task Post_heartbeat_with_malformed_json_returns_400()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-malformed", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        using var badContent = new StringContent("{not valid json", System.Text.Encoding.UTF8, "application/json");
        var resp = await _client.PostAsync($"{JobsUrl}/{jobId}/shards/{shardId}/heartbeat", badContent);

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    // ── 33. POST heartbeat with invalid orgId → 403 ──────────────────────────

    [Fact]
    public async Task Post_heartbeat_with_invalid_org_id_returns_403()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var fakeShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/not-an-org-id/coordinator/jobs/{fakeJobId}/shards/{fakeShardId}/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 34. POST heartbeat with unparseable jobId → 404 ─────────────────────

    [Fact]
    public async Task Post_heartbeat_with_unparseable_job_id_returns_404()
    {
        var fakeShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/not-a-job-id/shards/{fakeShardId}/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 35. POST heartbeat with unparseable shardId → 404 ───────────────────

    [Fact]
    public async Task Post_heartbeat_with_unparseable_shard_id_returns_404()
    {
        var fakeJobId = $"job_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{fakeJobId}/shards/not-a-shard-id/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── 36. POST heartbeat as non-member → 403 ───────────────────────────────

    [Fact]
    public async Task Post_heartbeat_returns_403_for_non_member()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-403-test", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        var claimResp = await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim",
            new { worker_id = "w1" });
        var claimBody = await claimResp.Content.ReadAsStringAsync();
        using var claimDoc = JsonDocument.Parse(claimBody);
        var shardId = claimDoc.RootElement.GetProperty("shard_id").GetString();

        var (nonMemberToken, _) = TestTokens.CreateNew("nonmember-hb@example.com");
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", nonMemberToken);

        var resp = await client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{shardId}/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── 37. POST heartbeat with unknown shardId → 404 ────────────────────────

    [Fact]
    public async Task Post_heartbeat_with_unknown_shard_id_returns_404()
    {
        var createResp = await _client.PostAsJsonAsync(JobsUrl,
            new { collection_sha = "hb-unknown-shard", shard_count = 1 });
        var jobId = await GetStringProp(createResp, "job_id");

        // Claim a shard so the job has a valid jobId
        await _client.PostAsJsonAsync($"{JobsUrl}/{jobId}/claim", new { worker_id = "w1" });

        var unknownShardId = $"shd_{Guid.NewGuid():N}";
        var resp = await _client.PostAsJsonAsync(
            $"{JobsUrl}/{jobId}/shards/{unknownShardId}/heartbeat",
            new { worker_id = "w1" });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }
}
