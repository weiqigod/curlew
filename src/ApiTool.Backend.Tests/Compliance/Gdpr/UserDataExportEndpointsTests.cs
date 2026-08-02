using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Storage;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;


namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>Integration tests for the GDPR user data export endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class UserDataExportEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _userId;

    public UserDataExportEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _userId = Guid.NewGuid();
        _client = factory.CreateClient();
        var token = TestTokens.Create(_userId, $"export-{_userId:N}@example.com");
        _client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private static async Task<string> ReadIdAsync(HttpResponseMessage response)
    {
        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        return doc.RootElement.GetProperty("id").GetString()!;
    }

    [Fact]
    public async Task POST_unauthenticated_returns_401()
    {
        using var anon = _factory.CreateClient();
        var response = await anon.PostAsync("/api/v1/users/me/export-requests", null);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task POST_first_call_returns_202_with_id_and_status_queued()
    {
        var userId = Guid.NewGuid();
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"post1-{userId:N}@example.com"));

        var response = await client.PostAsync("/api/v1/users/me/export-requests", null);
        response.StatusCode.Should().Be(HttpStatusCode.Accepted);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("id").GetString().Should().NotBeNullOrEmpty();
        doc.RootElement.GetProperty("status").GetString().Should().Be("queued");
    }

    [Fact]
    public async Task POST_second_call_within_24h_returns_429_with_Retry_After_header()
    {
        var userId = Guid.NewGuid();
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"rate-{userId:N}@example.com"));

        var first = await client.PostAsync("/api/v1/users/me/export-requests", null);
        first.StatusCode.Should().Be(HttpStatusCode.Accepted);

        var second = await client.PostAsync("/api/v1/users/me/export-requests", null);
        second.StatusCode.Should().Be(HttpStatusCode.TooManyRequests);
        second.Headers.Should().ContainKey("Retry-After");
    }

    [Fact]
    public async Task GET_unauthenticated_returns_401()
    {
        using var anon = _factory.CreateClient();
        var response = await anon.GetAsync($"/api/v1/users/me/export-requests/{Guid.NewGuid()}");
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task GET_unknown_id_returns_404()
    {
        var response = await _client.GetAsync($"/api/v1/users/me/export-requests/{Guid.NewGuid()}");
        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task GET_other_users_id_returns_404_no_enumeration()
    {
        // Other user posts a request
        var otherUserId = Guid.NewGuid();
        using var otherClient = _factory.CreateClient();
        otherClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(otherUserId, $"other-{otherUserId:N}@example.com"));

        var postResp = await otherClient.PostAsync("/api/v1/users/me/export-requests", null);
        postResp.StatusCode.Should().Be(HttpStatusCode.Accepted);
        var otherId = await ReadIdAsync(postResp);

        // Our client tries to GET the other user's request id
        var getResp = await _client.GetAsync($"/api/v1/users/me/export-requests/{otherId}");
        getResp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task GET_queued_row_returns_status_queued_without_signed_url()
    {
        var userId = Guid.NewGuid();
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"queued-{userId:N}@example.com"));

        var postResp = await client.PostAsync("/api/v1/users/me/export-requests", null);
        postResp.StatusCode.Should().Be(HttpStatusCode.Accepted);
        var id = await ReadIdAsync(postResp);

        var getResp = await client.GetAsync($"/api/v1/users/me/export-requests/{id}");
        getResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await getResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("status").GetString().Should().Be("queued");
        // signed_url is null/absent when not ready
        var hasSigned = doc.RootElement.TryGetProperty("signed_url", out var signedProp)
                        && signedProp.ValueKind != JsonValueKind.Null;
        hasSigned.Should().BeFalse("signed_url should not be present for a queued request");
    }

    [Fact]
    public async Task GET_ready_row_returns_signed_url_and_expires_at()
    {
        // Seed a ready row directly via DB to avoid test-isolation issues with
        // the shared BackendFactory's InMemory database. The object-store key is seeded
        // into the InMemoryObjectStore singleton so the signed URL can be resolved.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var store = scope.ServiceProvider.GetRequiredService<IObjectStore>() as InMemoryObjectStore;

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"ready2-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var reqId = Guid.NewGuid();
        var key = $"exports/{userId}/{reqId}.json";
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Ready,
            ObjectKey = key,
            CreatedAt = DateTime.UtcNow.AddMinutes(-2),
            ReadyAt = DateTime.UtcNow.AddMinutes(-1),
            ExpiresAt = DateTime.UtcNow.AddHours(23),
        });
        await db.SaveChangesAsync();

        // Seed the bundle into the in-memory store so GetSignedUrlAsync works.
        if (store is not null)
            await store.PutAsync(key, new MemoryStream("""{"tables":{}}"""u8.ToArray()), "application/json", default);

        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"ready2-{userId:N}@example.com"));

        var getResp = await client.GetAsync($"/api/v1/users/me/export-requests/{reqId}");
        getResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await getResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("status").GetString().Should().Be("ready");
        doc.RootElement.GetProperty("signed_url").GetString().Should().NotBeNullOrEmpty();
        doc.RootElement.GetProperty("expires_at").GetString().Should().NotBeNullOrEmpty();
    }

    [Fact]
    public async Task GET_failed_row_returns_status_failed_with_failure_reason()
    {
        // Seed a failed row directly via DB
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"fail-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var reqId = Guid.NewGuid();
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Failed,
            CreatedAt = DateTime.UtcNow,
            FailureReason = "InvalidOperationException",
        });
        await db.SaveChangesAsync();

        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"fail-{userId:N}@example.com"));

        var getResp = await client.GetAsync($"/api/v1/users/me/export-requests/{reqId}");
        getResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await getResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("status").GetString().Should().Be("failed");
        doc.RootElement.GetProperty("failure_reason").GetString().Should().Be("InvalidOperationException");
    }

    [Fact]
    public async Task Bundle_subset_matches_M18_003_manifest_InExport()
    {
        // Build a real bundle via the assembler and seed it into the in-memory store,
        // then seed a Ready row pointing to that key. Avoids the shared-DB race with TickOnceAsync.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var store = scope.ServiceProvider.GetRequiredService<IObjectStore>() as InMemoryObjectStore;

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"bundle2-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();

        // Assemble the real bundle.
        var manifest = GdprAttributeScanner.Manifest;
        var bundle = await UserExportBundleAssembler.AssembleAsync(db, manifest, userId, DateTime.UtcNow, default);
        var json = System.Text.Json.JsonSerializer.Serialize(bundle);

        var reqId = Guid.NewGuid();
        var key = $"exports/{userId}/{reqId}.json";
        if (store is not null)
            await store.PutAsync(key, new MemoryStream(System.Text.Encoding.UTF8.GetBytes(json)), "application/json", default);

        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Ready,
            ObjectKey = key,
            CreatedAt = DateTime.UtcNow.AddMinutes(-2),
            ReadyAt = DateTime.UtcNow.AddMinutes(-1),
            ExpiresAt = DateTime.UtcNow.AddHours(23),
        });
        await db.SaveChangesAsync();

        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"bundle2-{userId:N}@example.com"));

        var getResp = await client.GetAsync($"/api/v1/users/me/export-requests/{reqId}");
        var body = await getResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);

        var signedUrl = doc.RootElement.GetProperty("signed_url").GetString()!;

        // Fetch the bundle from the in-memory object store endpoint.
        var bundleResp = await client.GetAsync(signedUrl);
        bundleResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var bundleBody = await bundleResp.Content.ReadAsStringAsync();
        using var bundleDoc = JsonDocument.Parse(bundleBody);
        var tables = bundleDoc.RootElement.GetProperty("tables");
        var tableKeys = tables.EnumerateObject().Select(p => p.Name).ToList();
        tableKeys.Count.Should().Be(7, "the bundle should contain exactly the 7 InExport tables from M18-003+M18-005");

        // GDPR-critical: password_hash must never appear in the export bundle.
        // The user row was seeded without a password hash, so users[0] should exist
        // but must NOT carry password_hash.
        var usersArray = tables.GetProperty("users");
        usersArray.GetArrayLength().Should().Be(1, "exactly one user row was seeded");
        var userRow = usersArray[0];
        userRow.TryGetProperty("password_hash", out _).Should().BeFalse(
            "password_hash is a secret at rest and must be filtered from the export bundle");
        // Also verify token_hash is absent from refresh_tokens (0 rows expected here but
        // the sensitive-column filter should be applied regardless).
        var refreshTokensArray = tables.GetProperty("refresh_tokens");
        foreach (var tokenRow in refreshTokensArray.EnumerateArray())
        {
            tokenRow.TryGetProperty("token_hash", out _).Should().BeFalse(
                "token_hash must be filtered from the export bundle");
        }
    }

    [Fact]
    public async Task Rate_limit_429_body_carries_problem_detail_code()
    {
        var userId = Guid.NewGuid();
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"rl-{userId:N}@example.com"));

        await client.PostAsync("/api/v1/users/me/export-requests", null);
        var second = await client.PostAsync("/api/v1/users/me/export-requests", null);
        second.StatusCode.Should().Be(HttpStatusCode.TooManyRequests);

        var body = await second.Content.ReadAsStringAsync();
        body.Should().Contain("export_rate_limited");
    }

    [Fact]
    public async Task GET_expired_row_transitions_status_to_expired()
    {
        // Seed a row whose ExpiresAt is in the past so the GET endpoint marks it Expired.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"exp-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        var reqId = Guid.NewGuid();
        var key = $"exports/{userId}/{reqId}.json";
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = reqId,
            UserId = userId,
            Status = UserExportStatus.Ready,
            ObjectKey = key,
            CreatedAt = DateTime.UtcNow.AddHours(-26),
            ReadyAt = DateTime.UtcNow.AddHours(-25),
            ExpiresAt = DateTime.UtcNow.AddHours(-1), // already elapsed
        });
        await db.SaveChangesAsync();

        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"exp-{userId:N}@example.com"));

        var getResp = await client.GetAsync($"/api/v1/users/me/export-requests/{reqId}");
        getResp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await getResp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        // Status must be serialised as lowercase "expired" (via ToString().ToLowerInvariant())
        doc.RootElement.GetProperty("status").GetString().Should().Be("expired",
            "when ExpiresAt has elapsed the endpoint sets Status=Expired and serialises it as lowercase");
        // signed_url must be absent when expired
        var hasSignedUrl = doc.RootElement.TryGetProperty("signed_url", out var urlProp)
                           && urlProp.ValueKind != JsonValueKind.Null;
        hasSignedUrl.Should().BeFalse("expired rows must not return a signed_url");
    }

    [Fact]
    public async Task POST_second_call_after_24h_returns_202_again()
    {
        // Seed a request older than 24h directly via DB
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"old-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            Status = UserExportStatus.Ready,
            CreatedAt = DateTime.UtcNow.AddHours(-25), // 25h ago — past the rate-limit window
            ReadyAt = DateTime.UtcNow.AddHours(-24),
            ExpiresAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, $"old-{userId:N}@example.com"));

        var response = await client.PostAsync("/api/v1/users/me/export-requests", null);
        response.StatusCode.Should().Be(HttpStatusCode.Accepted,
            "a 25h-old request should be outside the 24h window so a new request is allowed");
    }
}
