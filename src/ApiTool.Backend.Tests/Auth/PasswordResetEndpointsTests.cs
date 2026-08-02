// Integration tests for POST /api/v1/auth/password-reset/{request,confirm}.
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

[Collection(BackendCollection.Name)]
public sealed class PasswordResetEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public PasswordResetEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _client  = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<User> SeedUserAsync(string? email = null)
    {
        email ??= $"reset-{Guid.NewGuid():N}@example.com";
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var hasher = scope.ServiceProvider.GetRequiredService<PasswordHasher>();
        var user = new User
        {
            Id = Guid.NewGuid(),
            Email = email,
            CreatedAt = DateTime.UtcNow,
            PasswordHash = hasher.Hash("OriginalPass1!"),
            EmailVerified = false,
        };
        db.Users.Add(user);
        await db.SaveChangesAsync();
        return user;
    }

    private RecordingEmailQueue GetEmailQueue() =>
        _factory.Services.GetRequiredService<RecordingEmailQueue>();

    // ── POST /api/v1/auth/password-reset/request ─────────────────────────────

    [Fact]
    public async Task POST_request_unknown_email_returns_200_with_standard_body()
    {
        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/request",
            new { email = "nobody@example.com" });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("ok").GetBoolean().Should().BeTrue();
        doc.RootElement.GetProperty("message").GetString().Should()
            .Contain("If an account exists");
    }

    [Fact]
    public async Task POST_request_known_email_returns_200_and_inserts_password_reset_token_row()
    {
        var user = await SeedUserAsync();

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/request",
            new { email = user.Email });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var tokenRows = db.PasswordResetTokens.Where(t => t.UserId == user.Id).ToList();
        tokenRows.Should().ContainSingle();
        tokenRows[0].ExpiresAt.Should().BeCloseTo(tokenRows[0].IssuedAt.AddMinutes(30), TimeSpan.FromSeconds(5));
    }

    [Fact]
    public async Task POST_request_known_email_enqueues_password_reset_email_with_reset_url()
    {
        var queue = GetEmailQueue();
        queue.Clear();

        var user = await SeedUserAsync();

        await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/request",
            new { email = user.Email });

        var emails = queue.Messages.Where(m => m.To == user.Email && m.TemplateSlug == "password_reset").ToList();
        emails.Should().ContainSingle();
        emails[0].Variables["reset_url"].Should().Contain("prst_");
    }

    [Fact]
    public async Task POST_request_fourth_within_24h_returns_200_but_does_not_enqueue_email()
    {
        var user  = await SeedUserAsync();
        var queue = GetEmailQueue();
        queue.Clear();

        // 3 allowed requests
        for (var i = 0; i < 3; i++)
            await _client.PostAsJsonAsync("/api/v1/auth/password-reset/request", new { email = user.Email });

        queue.Clear();

        // 4th should be silently rate-limited
        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/request",
            new { email = user.Email });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        queue.Messages.Where(m => m.To == user.Email).Should().BeEmpty();
    }

    // ── POST /api/v1/auth/password-reset/confirm ─────────────────────────────

    private async Task<string> SeedTokenAsync(User user)
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.PasswordResetTokens.Add(new PasswordResetToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt  = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddMinutes(30),
        });
        await db.SaveChangesAsync();
        return plaintext;
    }

    [Fact]
    public async Task POST_confirm_with_valid_token_and_strong_password_returns_200_and_revokes_refresh_families()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedTokenAsync(user);

        // Seed a refresh token
        {
            using var scope = _factory.Services.CreateScope();
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.RefreshTokens.Add(new RefreshToken
            {
                Id = Guid.NewGuid(), UserId = user.Id, DeviceId = Guid.NewGuid(), FamilyId = Guid.NewGuid(),
                TokenHash = AuthTokenIssuer.Hash($"rt-{Guid.NewGuid()}"),
                IssuedAt = DateTime.UtcNow.AddDays(-1), ExpiresAt = DateTime.UtcNow.AddDays(89),
            });
            await db.SaveChangesAsync();
        }

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/confirm",
            new { token = plaintext, new_password = "correct horse battery staple" });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var rt = verifyDb.RefreshTokens.Where(r => r.UserId == user.Id).Single();
        rt.RevokedAt.Should().NotBeNull();
        rt.RevokeReason.Should().Be("password_reset");
    }

    [Fact]
    public async Task POST_confirm_with_expired_token_returns_400_with_token_invalid_problem_detail()
    {
        var user = await SeedUserAsync();
        // Seed an already-expired token
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        {
            using var scope = _factory.Services.CreateScope();
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.PasswordResetTokens.Add(new PasswordResetToken
            {
                Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
                IssuedAt  = DateTime.UtcNow.AddHours(-2),
                ExpiresAt = DateTime.UtcNow.AddHours(-1), // expired
            });
            await db.SaveChangesAsync();
        }

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/confirm",
            new { token = plaintext, new_password = "correct horse battery staple" });

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("password-reset-token-invalid");
    }

    [Fact]
    public async Task POST_confirm_with_consumed_token_returns_400_with_token_invalid_problem_detail()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        {
            using var scope = _factory.Services.CreateScope();
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.PasswordResetTokens.Add(new PasswordResetToken
            {
                Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
                IssuedAt   = DateTime.UtcNow,
                ExpiresAt  = DateTime.UtcNow.AddMinutes(30),
                ConsumedAt = DateTime.UtcNow.AddMinutes(-1), // already consumed
            });
            await db.SaveChangesAsync();
        }

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/confirm",
            new { token = plaintext, new_password = "correct horse battery staple" });

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("password-reset-token-invalid");
    }

    [Fact]
    public async Task POST_confirm_with_weak_password_returns_422_with_score_field()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedTokenAsync(user);

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/password-reset/confirm",
            new { token = plaintext, new_password = "password" });

        response.StatusCode.Should().Be(HttpStatusCode.UnprocessableEntity);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("password-too-weak");
        json.Should().Contain("score");
    }
}
