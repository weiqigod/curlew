// Integration tests for the RequireVerifiedEmail endpoint filter.
using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

[Collection(BackendCollection.Name)]
public sealed class RequireVerifiedEmailFilterTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public RequireVerifiedEmailFilterTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<(HttpClient client, Guid userId)> CreateClientAsync(
        bool emailVerified, string? email = null)
    {
        var userId    = Guid.NewGuid();
        email       ??= $"filter-{userId:N}@example.com";
        var token     = TestTokens.Create(userId, email);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Users.Add(new User
        {
            Id = userId, Email = email,
            CreatedAt = DateTime.UtcNow,
            EmailVerified = emailVerified,
            PasswordHash = "$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA==$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
        });

        // Seed a basic org + subscription so checkout can reach the filter (not fail on org-not-found first)
        var org = new Organization
        {
            Id = Guid.NewGuid(), Name = "TestOrg",
            Slug = $"test-{userId:N}"[..20],
            OwnerId = userId, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
            Status = OrgStatus.Active,
            SettingsJson = "{}",
        };
        db.Organizations.Add(org);
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
        return (client, userId);
    }

    [Fact]
    public async Task Unverified_user_calling_checkout_receives_403_with_email_not_verified_type()
    {
        var (client, userId) = await CreateClientAsync(emailVerified: false);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var org = db.Organizations.First(o => o.OwnerId == userId);

        var response = await client.PostAsJsonAsync(
            "/api/v1/subscriptions/checkout",
            new { org_id = org.Id.ToString(), tier = "team", interval = "month",
                  seat_count = 1, success_url = "http://ok", cancel_url = "http://no" });

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("email-not-verified");
    }

    [Fact]
    public async Task Verified_user_calling_checkout_passes_filter()
    {
        var (client, userId) = await CreateClientAsync(emailVerified: true);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var org = db.Organizations.First(o => o.OwnerId == userId);

        var response = await client.PostAsJsonAsync(
            "/api/v1/subscriptions/checkout",
            new { org_id = org.Id.ToString(), tier = "team", interval = "month",
                  seat_count = 1, success_url = "http://ok", cancel_url = "http://no" });

        // Verified users are not blocked by the filter — may get other errors (400, etc.) but not 403
        response.StatusCode.Should().NotBe(HttpStatusCode.Forbidden);
    }

    [Fact]
    public async Task Anonymous_call_receives_401()
    {
        var anonClient = _factory.CreateClient();

        var response = await anonClient.PostAsJsonAsync(
            "/api/v1/subscriptions/checkout",
            new { org_id = Guid.NewGuid().ToString(), tier = "team", interval = "month",
                  seat_count = 1, success_url = "http://ok", cancel_url = "http://no" });

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Unverified_user_calling_invitation_accept_receives_403()
    {
        var (client, _) = await CreateClientAsync(emailVerified: false);

        var response = await client.PostAsJsonAsync(
            "/api/v1/invitations/accept",
            new { token = "some-token" });

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        json.Should().Contain("email-not-verified");
    }
}
