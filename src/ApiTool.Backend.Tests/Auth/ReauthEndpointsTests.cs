using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>
/// Integration tests for <c>POST /api/v1/auth/reauth</c> endpoint.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class ReauthEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    private const string ValidPassword = "Hunter2IsNotAPassword!";

    public ReauthEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<(User user, string jwt)> SeedUserAsync(string? password = ValidPassword)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var hasher = scope.ServiceProvider.GetRequiredService<PasswordHasher>();

        var userId = Guid.NewGuid();
        var email = $"reauth-{userId:N}@example.com";
        var user = new User
        {
            Id = userId,
            Email = email,
            CreatedAt = DateTime.UtcNow,
            PasswordHash = password is null ? null : hasher.Hash(password),
        };
        db.Users.Add(user);
        await db.SaveChangesAsync();
        return (user, TestTokens.Create(userId, email));
    }

    [Fact]
    public async Task POST_reauth_unauthenticated_returns_401()
    {
        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/reauth",
            new { password = ValidPassword });

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task POST_reauth_with_wrong_password_returns_401()
    {
        var (_, jwt) = await SeedUserAsync();
        _client.DefaultRequestHeaders.Authorization =
            new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", jwt);

        var response = await _client.PostAsJsonAsync(
            "/api/v1/auth/reauth",
            new { password = "WrongPasswordHere!" });

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        // Reset auth header
        _client.DefaultRequestHeaders.Authorization = null;
    }

    [Fact]
    public async Task POST_reauth_returns_200_with_drto_token_and_expires_at()
    {
        var (_, jwt) = await SeedUserAsync();
        using var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", jwt);

        var response = await client.PostAsJsonAsync(
            "/api/v1/auth/reauth",
            new { password = ValidPassword });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var reautToken = doc.RootElement.GetProperty("reauth_token").GetString();
        reautToken.Should().NotBeNull();
        reautToken!.Should().StartWith("drto_");

        doc.RootElement.TryGetProperty("expires_at", out _).Should().BeTrue();
    }
}
