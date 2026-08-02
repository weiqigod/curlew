using System.IdentityModel.Tokens.Jwt;
using System.Net;
using System.Net.Http.Headers;
using System.Security.Claims;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Licensing.Tokens;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>Verifies JWT authentication on the organizations endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class JwtAuthenticationTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;

    public JwtAuthenticationTests(BackendFactory factory)
    {
        _factory = factory;
        _client = factory.CreateClient();
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Requests_without_bearer_return_401_with_unauthorized_code()
    {
        var response = await _client.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        response.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("unauthorized");
        doc.RootElement.GetProperty("status").GetInt32().Should().Be(401);
    }

    [Fact]
    public async Task Requests_with_invalid_signature_return_401_with_unauthorized_code()
    {
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", "not.a.valid.token");

        var response = await _client.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("unauthorized");
    }

    [Fact]
    public async Task Valid_token_upserts_missing_user_row()
    {
        var (token, _) = TestTokens.CreateNew("upsert@example.com");
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        var response = await _client.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Cli_access_token_is_accepted_by_bearer_authentication()
    {
        var userId = Guid.NewGuid();

        // First establish the user through the browser/session-token path. Access tokens
        // deliberately do not repeat profile data such as email.
        var sessionClient = _factory.CreateClient();
        sessionClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(userId, "cli-access@example.com"));
        (await sessionClient.GetAsync("/api/v1/organizations"))
            .StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var issuer = scope.ServiceProvider.GetRequiredService<AccessTokenIssuer>();
        var accessToken = await issuer.IssueAsync(new AccessTokenInput(
            UserId: userId,
            Tier: "team",
            OrgId: null,
            DeviceId: Guid.NewGuid()));

        var cliClient = _factory.CreateClient();
        cliClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", accessToken);

        var response = await cliClient.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Token_with_non_guid_sub_returns_401_with_unauthorized_code()
    {
        // Mint a structurally-valid HS256 JWT whose 'sub' claim is not a GUID.
        var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(BackendFactory.TestSigningKey));
        var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);
        var claims = new[]
        {
            new Claim(JwtRegisteredClaimNames.Sub, "not-a-guid"),
            new Claim(JwtRegisteredClaimNames.Email, "user@example.com"),
        };
        var token = new JwtSecurityTokenHandler().WriteToken(new JwtSecurityToken(
            issuer: BackendFactory.TestIssuer,
            audience: BackendFactory.TestAudience,
            claims: claims,
            expires: DateTime.UtcNow.AddHours(1),
            signingCredentials: credentials));

        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        var response = await _client.GetAsync("/api/v1/organizations");

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("unauthorized");
    }

    [Fact]
    public async Task ResolveAsync_succeeds_on_second_request_when_user_row_already_exists()
    {
        // Verifies that a repeat request from the same user (row already in DB) succeeds without
        // a double-insert error. CurrentUserAccessor is Scoped, so each request gets a fresh
        // instance — there is no within-instance cache; this covers the FindAsync-returns-non-null path.
        var (token, _) = TestTokens.CreateNew("repeat-user@example.com");
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);

        // First request — upserts the user row.
        var first = await client.GetAsync("/api/v1/organizations");
        first.StatusCode.Should().Be(HttpStatusCode.OK);

        // Second request — user row exists; FindAsync returns it and skips the insert.
        var second = await client.GetAsync("/api/v1/organizations");
        second.StatusCode.Should().Be(HttpStatusCode.OK);
    }
}
