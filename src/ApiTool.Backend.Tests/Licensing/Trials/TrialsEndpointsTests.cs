// Integration tests for POST /api/v1/trials/{feature}.
// Refs docs/SPECIFICATION.md:5800-5860 (on-demand trial activation).
using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Licensing.Trials;

[Collection(BackendCollection.Name)]
public sealed class TrialsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private HttpClient _client = null!;

    public TrialsEndpointsTests(BackendFactory f) => _factory = f;

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        _client = _factory.CreateClient();
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── Seeding helpers ───────────────────────────────────────────────────────

    /// <summary>Seeds a user with a refresh token and full-initial trials. Returns access JWT + plaintext RT + deviceId.</summary>
    private async Task<(Guid userId, string bearerToken, string refreshToken, Guid deviceId)> SeedUserAsync()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        var email  = $"trial-test-{userId:N}@example.com";
        db.Users.Add(new User { Id = userId, Email = email, CreatedAt = DateTime.UtcNow });

        var deviceId = Guid.NewGuid();
        var familyId = Guid.NewGuid();
        var issuer   = new RefreshTokenIssuer(TimeProvider.System);
        var (plaintext, row) = issuer.Mint(userId, deviceId, familyId,
            parentId: null, familyRootIssuedAt: DateTime.UtcNow, clientIp: null, userAgent: null);
        db.RefreshTokens.Add(row);
        await db.SaveChangesAsync();

        // Seed full-initial trial rows (matches production user-creation path).
        var seeder = new TrialSeederService(db, TimeProvider.System, NullLogger<TrialSeederService>.Instance);
        await seeder.SeedFullInitialAsync(userId, DateTime.UtcNow);

        var bearerToken = TestTokens.Create(userId, email);
        return (userId, bearerToken, plaintext, deviceId);
    }

    private async Task DeleteTrialRowAsync(Guid userId, string feature)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.Trials.FirstOrDefaultAsync(t => t.UserId == userId && t.Feature == feature);
        if (row is not null)
        {
            db.Trials.Remove(row);
            await db.SaveChangesAsync();
        }
    }

    // ── Tests ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task POST_unknown_feature_returns_404_with_problem_type()
    {
        var (_, bearer, rt, deviceId) = await SeedUserAsync();
        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);

        var resp = await _client.PostAsJsonAsync("/api/v1/trials/made_up_feature",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("TRIAL_FEATURE_UNKNOWN");
    }

    [Fact]
    public async Task POST_with_existing_full_initial_row_returns_409_with_previous_grant()
    {
        // A freshly-seeded user always has full-initial rows → always 409.
        var (_, bearer, rt, deviceId) = await SeedUserAsync();
        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);

        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("TRIAL_ALREADY_CONSUMED");
        body.TryGetProperty("previous_grant", out _).Should().BeTrue();
    }

    [Fact]
    public async Task POST_with_existing_ondemand_row_returns_409_with_previous_grant()
    {
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        // Delete full-initial, then insert an ondemand row directly.
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var now = DateTime.UtcNow;
            db.Trials.Add(new Trial
            {
                Id = Guid.NewGuid(), UserId = userId, Feature = "vault_provider_profiles",
                Kind = TrialKind.OnDemand, GrantedAt = now, ExpiresAt = now.AddDays(7),
                CreatedAt = now, UpdatedAt = now,
            });
            await db.SaveChangesAsync();
        }

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("TRIAL_ALREADY_CONSUMED");
    }

    [Fact]
    public async Task POST_first_time_grants_ondemand_and_inserts_row()
    {
        // Delete the full-initial row so the Granted path is reachable.
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify the row was inserted.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.Trials.FirstOrDefaultAsync(t =>
            t.UserId == userId && t.Feature == "vault_provider_profiles" && t.Kind == TrialKind.OnDemand);
        row.Should().NotBeNull();
    }

    [Fact]
    public async Task POST_first_time_response_carries_re_minted_tokens()
    {
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("tokens").GetProperty("license_jwt").GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("tokens").GetProperty("access_token").GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("tokens").GetProperty("refresh_token").GetString().Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task POST_first_time_response_has_correct_feature_and_kind()
    {
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("feature").GetString().Should().Be("vault_provider_profiles");
        body.GetProperty("kind").GetString().Should().Be("ondemand");
    }

    [Fact]
    public async Task POST_invalid_request_body_returns_400()
    {
        var (_, bearer, _, _) = await SeedUserAsync();
        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);

        // Missing device_id.
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = "some-token" });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task POST_unauthenticated_returns_401()
    {
        _client.DefaultRequestHeaders.Authorization = null;
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = "some-token", device_id = Guid.NewGuid() });

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task POST_first_time_license_jwt_includes_feature_in_features_claim()
    {
        // Behavior 4: the re-minted License JWT must list the activated feature in features[].
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body     = await resp.Content.ReadFromJsonAsync<JsonElement>();
        var jwt      = body.GetProperty("tokens").GetProperty("license_jwt").GetString()!;
        var payload  = DecodeJwsPayload(jwt);

        // features[] must contain "vault_provider_profiles".
        var features = payload.GetProperty("features").EnumerateArray()
            .Select(f => f.GetString())
            .ToList();
        features.Should().Contain("vault_provider_profiles",
            because: "the activated feature must appear in features[] of the re-minted License JWT (Behavior 4)");
    }

    [Fact]
    public async Task POST_first_time_license_jwt_carries_trial_state_active_and_trial_expiry()
    {
        // Behavior 7: the re-minted License JWT must carry trial_state = "active"
        // and a non-zero trial_expiry unix timestamp.
        var (userId, bearer, rt, deviceId) = await SeedUserAsync();
        await DeleteTrialRowAsync(userId, "vault_provider_profiles");

        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
        var resp = await _client.PostAsJsonAsync("/api/v1/trials/vault_provider_profiles",
            new { refresh_token = rt, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body    = await resp.Content.ReadFromJsonAsync<JsonElement>();
        var jwt     = body.GetProperty("tokens").GetProperty("license_jwt").GetString()!;
        var payload = DecodeJwsPayload(jwt);

        payload.GetProperty("trial_state").GetString().Should().Be("active",
            because: "a just-activated on-demand trial must produce trial_state=active (Behavior 7)");

        // trial_expiry must be a positive unix timestamp (~now + 7d).
        payload.TryGetProperty("trial_expiry", out var trialExpiry).Should().BeTrue(
            because: "trial_expiry must be present when trial_state=active");
        trialExpiry.GetInt64().Should().BeGreaterThan(0,
            because: "trial_expiry is a unix epoch second; must be non-zero");
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private static JsonElement DecodeJwsPayload(string jwt)
    {
        var parts       = jwt.Split('.');
        var payloadJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[1]));
        return JsonDocument.Parse(payloadJson).RootElement;
    }
}
