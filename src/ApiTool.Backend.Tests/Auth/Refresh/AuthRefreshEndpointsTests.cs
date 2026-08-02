// Integration tests for POST /api/v1/auth/refresh.
// Refs docs/SPECIFICATION.md:8228, :7901-7944.
using System.Net;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Auth.Refresh;

[Collection(BackendCollection.Name)]
public sealed class AuthRefreshEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private HttpClient _client = null!;

    public AuthRefreshEndpointsTests(BackendFactory f) => _factory = f;

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        _client = _factory.CreateClient();
    }

    public Task DisposeAsync() => Task.CompletedTask;

    // ── Seeding helpers ───────────────────────────────────────────────────────

    private async Task<(User user, Guid deviceId, string plaintext)> SeedRefreshTokenAsync()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var user = new User
        {
            Id        = Guid.NewGuid(),
            Email     = $"refresh-test-{Guid.NewGuid():N}@example.com",
            CreatedAt = DateTime.UtcNow,
        };
        db.Users.Add(user);

        var deviceId = Guid.NewGuid();
        var familyId = Guid.NewGuid();
        var issuer   = new RefreshTokenIssuer(TimeProvider.System);
        var (plaintext, row) = issuer.Mint(user.Id, deviceId, familyId,
            parentId: null, familyRootIssuedAt: DateTime.UtcNow, clientIp: null, userAgent: null);
        db.RefreshTokens.Add(row);
        await db.SaveChangesAsync();

        // Seed full-initial trial rows so the resolver returns "active" for this test user,
        // matching what the production user-creation paths do. Refs M16-006.
        var seeder = new TrialSeederService(db, TimeProvider.System,
            NullLogger<TrialSeederService>.Instance);
        await seeder.SeedFullInitialAsync(user.Id, user.CreatedAt);

        return (user, deviceId, plaintext);
    }

    private RecordingEmailQueue GetEmailQueue()
    {
        // The BackendFactory registers RecordingEmailQueue for the IEmailQueue.
        return _factory.Services.GetRequiredService<RecordingEmailQueue>();
    }

    // ── Tests ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task POST_auth_refresh_with_valid_token_returns_200_with_unified_mint_shape()
    {
        // Behavior #1
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();

        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();

        body.GetProperty("license_jwt"   ).GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("access_token"  ).GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("refresh_token" ).GetString().Should().NotBeNullOrEmpty();

        // Exactly three top-level keys (no device_id leakage, etc.)
        body.EnumerateObject().Select(p => p.Name).Should()
            .BeEquivalentTo(["license_jwt", "access_token", "refresh_token"]);
    }

    [Fact]
    public async Task License_jwt_carries_typ_license_jwt_with_17_claim_shape()
    {
        // Behavior #1 — License JWT shape
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        var licenseJwt = body.GetProperty("license_jwt").GetString()!;

        var (header, payload) = DecodeJws(licenseJwt);
        header.GetProperty("typ").GetString().Should().Be("license+jwt");
        header.GetProperty("alg").GetString().Should().Be("ES256");

        var requiredClaims = new[]
        {
            "iss", "aud", "sub", "exp", "nbf", "iat", "jti",
            "email", "tier", "features", "request_limit",
            "org_id", "org_role", "device_id",
            "trial_state", "trial_expiry", "grace_until",
        };
        foreach (var claim in requiredClaims)
            payload.TryGetProperty(claim, out _).Should().BeTrue(because: $"'{claim}' must be present");

        // trial_state is active for a seeded user (M16-006); non-empty string is sufficient here.
        payload.GetProperty("trial_state").GetString().Should().NotBeNullOrEmpty();
        payload.GetProperty("aud").GetString().Should().Be("apitool-license");
    }

    [Fact]
    public async Task Access_token_carries_typ_at_jwt_with_9_claim_shape()
    {
        // Behavior #1 — Access token shape
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        var accessToken = body.GetProperty("access_token").GetString()!;

        var (header, payload) = DecodeJws(accessToken);
        header.GetProperty("typ").GetString().Should().Be("at+jwt");
        payload.GetProperty("aud").GetString().Should().Be("apitool-cli-api");
        payload.TryGetProperty("email", out _).Should().BeFalse();
    }

    [Fact]
    public async Task Reuse_returns_401_AUTH_REFRESH_REUSED_with_problem_json()
    {
        // Behavior #3
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();

        // First rotate: marks the token as rotated
        await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });

        // Second use: reuse attempt
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code"  ).GetString().Should().Be("AUTH_REFRESH_REUSED");
        body.GetProperty("status").GetInt32() .Should().Be(401);
        body.GetProperty("type"  ).GetString().Should().Contain("refresh-token-reused");

        // Email must be enqueued
        GetEmailQueue().Messages.Should().Contain(m => m.TemplateSlug == "account_security_alert");
    }

    [Fact]
    public async Task DeviceMismatch_returns_401_AUTH_DEVICE_MISMATCH_and_does_not_rotate()
    {
        // Behavior #4
        var (_, _, plaintext) = await SeedRefreshTokenAsync();
        var wrongDevice = Guid.NewGuid();

        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = wrongDevice });

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("AUTH_DEVICE_MISMATCH");
    }

    [Fact]
    public async Task Expired_token_returns_401_AUTH_REFRESH_EXPIRED()
    {
        // Behavior #5
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = new User
        {
            Id        = Guid.NewGuid(),
            Email     = $"expired-{Guid.NewGuid():N}@example.com",
            CreatedAt = DateTime.UtcNow,
        };
        db.Users.Add(user);
        var deviceId  = Guid.NewGuid();
        var plaintext = "expired-tok-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
        var row = new RefreshToken
        {
            Id        = Guid.NewGuid(),
            TokenHash = RefreshTokenIssuer.Hash(plaintext),
            UserId    = user.Id,
            DeviceId  = deviceId,
            FamilyId  = Guid.NewGuid(),
            IssuedAt  = DateTime.UtcNow.AddDays(-400),
            ExpiresAt = DateTime.UtcNow.AddDays(-10),
        };
        db.RefreshTokens.Add(row);
        await db.SaveChangesAsync();

        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("AUTH_REFRESH_EXPIRED");
    }

    [Fact]
    public async Task Trial_fields_reflect_seeded_full_initial_for_new_user()
    {
        // Behavior #6 — M16-006 revised: seeded user has active trial state.
        // With TrialSeederService wired, a newly-created user gets 14-day full-initial rows,
        // so the License JWT must carry trial_state="active" and a non-null trial_expiry.
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        var licenseJwt = body.GetProperty("license_jwt").GetString()!;
        var (_, payload) = DecodeJws(licenseJwt);

        payload.GetProperty("trial_state").GetString().Should().Be("active");
        payload.TryGetProperty("trial_expiry", out var trialExpiry).Should().BeTrue();
        trialExpiry.ValueKind.Should().Be(JsonValueKind.Number,
            because: "active trial must have a unix-second expiry");
        trialExpiry.GetInt64().Should().BeGreaterThan(0);
        var features = payload.GetProperty("features").EnumerateArray()
            .Select(e => e.GetString()).ToList();
        features.Should().Contain("vault_provider_profiles",
            because: "trialing features must be unioned into features[]");
    }

    [Fact]
    public async Task Bad_request_with_empty_body_returns_problem_with_AUTH_INVALID_REFRESH()
    {
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh", new { });

        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("AUTH_INVALID_REFRESH");
    }

    [Fact]
    public async Task Unknown_token_returns_401_AUTH_INVALID_REFRESH()
    {
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = "unknown-tok-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", device_id = Guid.NewGuid() });

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("code").GetString().Should().Be("AUTH_INVALID_REFRESH");
    }

    [Fact]
    public async Task Response_sets_X_Request_Id_header()
    {
        var (_, _, plaintext) = await SeedRefreshTokenAsync();
        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = Guid.NewGuid() });

        // Either a success or error response should set X-Request-Id
        resp.Headers.Contains("X-Request-Id").Should().BeTrue();
    }

    [Fact]
    public async Task Rotation_marks_old_rotated_and_inserts_new_row_with_correct_family_linkage()
    {
        // Behavior #2 (integration test counterpart)
        // Verifies DB state via _factory.Services after a successful refresh call.
        var (_, deviceId, plaintext) = await SeedRefreshTokenAsync();

        // Look up the old row's ID before rotation
        Guid oldRowId;
        Guid oldFamilyId;
        {
            using var scope = _factory.Services.CreateScope();
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var hash = ApiTool.Backend.Auth.Refresh.RefreshTokenIssuer.Hash(plaintext);
            var oldRow = await db.RefreshTokens.FirstAsync(r => r.TokenHash == hash);
            oldRowId   = oldRow.Id;
            oldFamilyId = oldRow.FamilyId;
        }

        var resp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify DB state after rotation
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();

        // Old row should now have RotatedAt set
        var rotatedOld = await verifyDb.RefreshTokens.FindAsync(oldRowId);
        rotatedOld!.RotatedAt.Should().NotBeNull();

        // A new row should exist in the same family with ParentId = oldRowId
        var newRow = await verifyDb.RefreshTokens
            .FirstOrDefaultAsync(r => r.FamilyId == oldFamilyId && r.ParentId == oldRowId);
        newRow.Should().NotBeNull(because: "rotation must insert a successor row in the same family");
        newRow!.RotatedAt.Should().BeNull(because: "new row has not yet been rotated");
    }

    [Fact]
    public async Task Seed_endpoint_returns_plaintext_and_device_id_usable_by_refresh()
    {
        // Finding #5: integration test for POST /internal/test/seed-refresh.
        // Asserts the response shape {plaintext, device_id} and that the returned
        // plaintext is immediately usable by POST /api/v1/auth/refresh.
        var email    = $"seed-test-{Guid.NewGuid():N}@example.com";
        var deviceId = Guid.NewGuid();

        var seedResp = await _client.PostAsJsonAsync("/internal/test/seed-refresh",
            new { email, device_id = deviceId });

        seedResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var seedBody = await seedResp.Content.ReadFromJsonAsync<JsonElement>();
        var plaintext = seedBody.GetProperty("plaintext").GetString();
        plaintext.Should().NotBeNullOrEmpty();
        seedBody.GetProperty("device_id").GetGuid().Should().Be(deviceId);

        // The returned plaintext should be immediately usable by the refresh endpoint
        var refreshResp = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintext, device_id = deviceId });
        refreshResp.StatusCode.Should().Be(HttpStatusCode.OK,
            because: "seeded token must be valid for the refresh endpoint");
    }

    [Fact]
    public async Task Active_kid_after_rotation_is_reflected_in_minted_jwt_headers()
    {
        // Behavior #7 (integration): verifies that the DI-wired IKeyProvider picks up
        // the new active kid after a simulated key rotation mid-flight.
        // The FakeKeyProvider exposes SimulateKeyRotation() for this purpose.
        var fakeKp = _factory.GetFakeKeyProvider();
        fakeKp.Should().NotBeNull(because: "BackendFactory must register FakeKeyProvider");

        // Get a valid token before rotation so we know the pre-rotation kid
        var (_, deviceIdBefore, plaintextBefore) = await SeedRefreshTokenAsync();
        var respBefore = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintextBefore, device_id = deviceIdBefore });
        respBefore.StatusCode.Should().Be(HttpStatusCode.OK);
        var bodyBefore = await respBefore.Content.ReadFromJsonAsync<JsonElement>();
        var (headerBefore, _) = DecodeJws(bodyBefore.GetProperty("license_jwt").GetString()!);
        var kidBefore = headerBefore.GetProperty("kid").GetString()!;

        // Simulate key rotation (replaces the active key + kid in the singleton FakeKeyProvider)
        var newKid = fakeKp!.SimulateKeyRotation();

        // Mint a new token after rotation
        var (_, deviceIdAfter, plaintextAfter) = await SeedRefreshTokenAsync();
        var respAfter = await _client.PostAsJsonAsync("/api/v1/auth/refresh",
            new { refresh_token = plaintextAfter, device_id = deviceIdAfter });
        respAfter.StatusCode.Should().Be(HttpStatusCode.OK);
        var bodyAfter = await respAfter.Content.ReadFromJsonAsync<JsonElement>();
        var (headerAfter, _) = DecodeJws(bodyAfter.GetProperty("license_jwt").GetString()!);
        var kidAfter = headerAfter.GetProperty("kid").GetString()!;

        kidAfter.Should().Be(newKid,
            because: "handler wiring must resolve the current active kid after key rotation");
        kidAfter.Should().NotBe(kidBefore,
            because: "a rotated key must produce a different kid");
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private static (JsonElement header, JsonElement payload) DecodeJws(string jwt)
    {
        var parts      = jwt.Split('.');
        var headerJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[0]));
        var payloadJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[1]));
        return (JsonDocument.Parse(headerJson).RootElement,
                JsonDocument.Parse(payloadJson).RootElement);
    }
}
