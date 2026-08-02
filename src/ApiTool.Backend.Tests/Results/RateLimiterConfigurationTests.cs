using System.Net;
using System.Net.Http.Headers;
using System.Text;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Results;

/// <summary>
/// Verifies that the rate-limiter policy "results-ingest" is registered and that
/// the POST /results endpoint is reachable (not 503/500 due to a missing policy).
/// The Testing environment uses a no-limiter policy so fixtures cannot flake;
/// production uses a 60 req/min fixed-window policy per org.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class RateLimiterConfigurationTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public RateLimiterConfigurationTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"rl-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);
        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        var slug = $"rl-{_ownerId:N}"[..20];
        var body = new System.Net.Http.StringContent(
            System.Text.Json.JsonSerializer.Serialize(new { name = "RateLimiterTestOrg", slug }),
            Encoding.UTF8, "application/json");
        var response = await _client.PostAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = System.Text.Json.JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task RateLimiter_policy_results_ingest_is_registered_and_endpoint_is_reachable()
    {
        // If the "results-ingest" policy were missing, ASP.NET Core would throw an
        // InvalidOperationException at startup (or on the first request to the route).
        // A successful response here proves the policy is registered and bound correctly.
        var payload = new System.Net.Http.StringContent(
            System.Text.Json.JsonSerializer.Serialize(new
            {
                collection_name = "rate-limiter-probe",
                run_at = "2026-04-15T12:00:00Z",
                duration_ms = 1,
                pass_count = 1,
                fail_count = 0,
                items = new[] { new { name = "probe", status = "passed", duration_ms = 1 } },
            }),
            Encoding.UTF8, "application/json");

        var response = await _client.PostAsync($"/api/v1/organizations/{_orgId}/results", payload);

        // 202 Accepted proves: app started, policy registered, endpoint bound, RBAC passed.
        // In the Testing environment the no-limiter policy is used so we never get 429.
        response.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "the results-ingest rate-limiter policy must be registered and the endpoint must be reachable");
    }
}
