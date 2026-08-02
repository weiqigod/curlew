using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Invitations;

/// <summary>
/// Regression for the seed-enterprise.sh acceptance flow: a second user logging in
/// via JWT must have its email claim persisted to the <c>users</c> table. Previously
/// <see cref="ApiTool.Backend.Auth.CurrentUserAccessor"/> looked up the email under
/// <c>JwtRegisteredClaimNames.Email</c> but JWT bearer middleware maps that to
/// <c>ClaimTypes.Email</c>, so every user row was saved with an empty email —
/// hidden by the InMemory provider's lack of UNIQUE-index enforcement.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class AcceptInvitationPersistsEmailTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public AcceptInvitationPersistsEmailTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Accepting_user_row_has_email_from_jwt_claim()
    {
        var ownerId = Guid.NewGuid();
        var ownerEmail = $"owner-{ownerId:N}@example.com";
        var ownerClient = _factory.CreateClient();
        ownerClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", TestTokens.Create(ownerId, ownerEmail));

        var slug = $"em-{Guid.NewGuid():N}"[..20];
        var orgResp = await ownerClient.PostAsJsonAsync("/api/v1/organizations",
            new { name = "EmailOrg", slug });
        orgResp.StatusCode.Should().Be(HttpStatusCode.Created);
        var orgId = JsonDocument.Parse(await orgResp.Content.ReadAsStringAsync())
            .RootElement.GetProperty("id").GetString()!;

        // M16-003: checkout is gated on email_verified.
        await _factory.SetEmailVerifiedAsync(ownerId);
        await ownerClient.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://x", cancel_url = "http://y",
        });

        var acceptEmail = $"accept-{Guid.NewGuid():N}@example.com";
        var inviteResp = await ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations",
            new { email = acceptEmail, role = "member" });
        inviteResp.StatusCode.Should().Be(HttpStatusCode.Created);
        var inviteToken = JsonDocument.Parse(await inviteResp.Content.ReadAsStringAsync())
            .RootElement.GetProperty("token").GetString()!;

        var (accepteeToken, accepteeId) = TestTokens.CreateNew(acceptEmail);
        var acceptClient = _factory.CreateClient();
        acceptClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", accepteeToken);

        // M16-003: invite-accept is gated on email_verified.
        await acceptClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(accepteeId);

        var acceptResp = await acceptClient.PostAsJsonAsync("/api/v1/invitations/accept",
            new { token = inviteToken });
        acceptResp.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var ownerRow = await db.Users.AsNoTracking().SingleAsync(u => u.Id == ownerId);
        var accepteeRow = await db.Users.AsNoTracking().SingleAsync(u => u.Id == accepteeId);

        ownerRow.Email.Should().Be(ownerEmail,
            "CurrentUserAccessor must persist the JWT email claim, not an empty string");
        accepteeRow.Email.Should().Be(acceptEmail);
    }
}
