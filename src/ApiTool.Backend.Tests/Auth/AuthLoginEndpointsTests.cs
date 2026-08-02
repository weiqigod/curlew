using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>Integration tests for the local-password auth login endpoint.</summary>
[Collection(BackendCollection.Name)]
public sealed class AuthLoginEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public AuthLoginEndpointsTests(BackendFactory f) => _factory = f;

    public async Task InitializeAsync() => await _factory.InitializeAsync();

    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Login_with_valid_admin_credentials_returns_200_and_access_token()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var hasher = new PasswordHasher();
        db.Users.Add(new User
        {
            Id           = Guid.NewGuid(),
            Email        = "login-admin@example.com",
            CreatedAt    = DateTime.UtcNow,
            PasswordHash = hasher.Hash("ChangeMe!Password"),
            IsAdmin      = true,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "login-admin@example.com", password = "ChangeMe!Password" });

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("access_token").GetString().Should().NotBeNullOrEmpty();
        body.GetProperty("token_type").GetString().Should().Be("Bearer");
        body.GetProperty("role").GetString().Should().Be("admin");
    }

    [Fact]
    public async Task Login_with_unknown_email_returns_401_invalid_credentials()
    {
        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "nobody@example.com", password = "any-password-1234" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Login_with_wrong_password_returns_401_invalid_credentials()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Users.Add(new User
        {
            Id           = Guid.NewGuid(),
            Email        = "pw-test@example.com",
            CreatedAt    = DateTime.UtcNow,
            PasswordHash = new PasswordHasher().Hash("Correct!Password"),
            IsAdmin      = false,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "pw-test@example.com", password = "Wrong!Password" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Login_with_empty_body_returns_400()
    {
        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login", new { });
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Login_for_sso_only_user_without_password_returns_401()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        db.Users.Add(new User
        {
            Id        = Guid.NewGuid(),
            Email     = "sso-only@example.com",
            CreatedAt = DateTime.UtcNow,
            PasswordHash = null,
        });
        await db.SaveChangesAsync();

        var client = _factory.CreateClient();
        var resp = await client.PostAsJsonAsync("/api/v1/auth/login",
            new { email = "sso-only@example.com", password = "any-password-1234" });
        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }
}
