// Integration tests for POST /api/v1/auth/email-verification/{resend,confirm}.
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
public sealed class EmailVerificationEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public EmailVerificationEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _client  = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<User> SeedUserAsync(string? email = null)
    {
        email ??= $"evtk-{Guid.NewGuid():N}@example.com";
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var user = new User
        {
            Id = Guid.NewGuid(), Email = email,
            CreatedAt = DateTime.UtcNow, EmailVerified = false,
        };
        db.Users.Add(user);
        await db.SaveChangesAsync();
        return user;
    }

    private RecordingEmailQueue GetEmailQueue() =>
        _factory.Services.GetRequiredService<RecordingEmailQueue>();

    // ── POST /api/v1/auth/email-verification/resend ───────────────────────────

    [Fact]
    public async Task POST_resend_unknown_email_returns_200_with_standard_body()
    {
        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/email-verification/resend",
            new { email = "nobody@example.com" });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("ok").GetBoolean().Should().BeTrue();
    }

    [Fact]
    public async Task POST_resend_known_unverified_email_inserts_evtk_row_and_revokes_prior_rows()
    {
        var user  = await SeedUserAsync();
        var queue = GetEmailQueue();
        queue.Clear();

        // First resend
        await _client.PostAsJsonAsync("/api/v1/auth/email-verification/resend", new { email = user.Email });

        // Second resend should revoke first token
        queue.Clear();
        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/email-verification/resend",
            new { email = user.Email });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var tokens = db.EmailVerificationTokens.Where(t => t.UserId == user.Id).ToList();
        tokens.Should().HaveCount(2);
        tokens.Count(t => t.RevokedAt != null).Should().Be(1);
        tokens.Count(t => t.RevokedAt == null && t.ConsumedAt == null).Should().Be(1);

        queue.Messages.Should().ContainSingle(m =>
            m.To == user.Email && m.TemplateSlug == "email_verification");
    }

    // ── POST /api/v1/auth/email-verification/confirm ──────────────────────────

    private async Task<string> SeedTokenAsync(User user)
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.EmailVerificationTokens.Add(new EmailVerificationToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
            IssuedAt  = DateTime.UtcNow,
            ExpiresAt = DateTime.UtcNow.AddHours(24),
        });
        await db.SaveChangesAsync();
        return plaintext;
    }

    [Fact]
    public async Task POST_confirm_valid_token_sets_email_verified_true()
    {
        var user      = await SeedUserAsync();
        var plaintext = await SeedTokenAsync(user);

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/email-verification/confirm",
            new { token = plaintext });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.Users.FindAsync(user.Id);
        updated!.EmailVerified.Should().BeTrue();
    }

    [Fact]
    public async Task POST_confirm_expired_token_returns_400_with_problem_detail()
    {
        var user = await SeedUserAsync();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        {
            using var scope = _factory.Services.CreateScope();
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.EmailVerificationTokens.Add(new EmailVerificationToken
            {
                Id = Guid.NewGuid(), UserId = user.Id, TokenHash = hash,
                IssuedAt  = DateTime.UtcNow.AddHours(-25),
                ExpiresAt = DateTime.UtcNow.AddHours(-1), // expired
            });
            await db.SaveChangesAsync();
        }

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/email-verification/confirm",
            new { token = plaintext });

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("email-verification-token-invalid");
    }
}
