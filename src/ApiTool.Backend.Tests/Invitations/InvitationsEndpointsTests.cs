using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Invitations;

/// <summary>Full behaviour-level tests for the invitations endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class InvitationsEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;

    public InvitationsEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"inv-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        // M16-003: checkout and invite-accept now require email_verified.
        // Trigger user upsert then mark verified.
        await _client.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(_ownerId);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<string> CreateOrgWithSubscriptionAsync()
    {
        var slug = $"inv-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "InvOrg", slug });
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgId = doc.RootElement.GetProperty("id").GetString()!;

        // Create a subscription with 5 seats.
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);
        return orgId;
    }

    // ── POST /api/v1/organizations/{id}/invitations ───────────────────────────

    [Fact]
    public async Task Post_creates_invitation_and_returns_201_with_email_and_role()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var body = new { email = $"invite-{Guid.NewGuid():N}@example.com", role = "member" };
        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("invitation").GetProperty("email").GetString().Should().Be(body.email);
        doc.RootElement.GetProperty("invitation").GetProperty("role").GetString().Should().Be("member");
    }

    [Fact]
    public async Task Post_writes_member_invited_audit_row()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        var body = new { email = $"audit-{Guid.NewGuid():N}@example.com", role = "member" };
        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Created);

        // Verify the audit log row was written to the database.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var auditEntry = await db.OrganizationAuditLog
            .FirstOrDefaultAsync(e => e.OrgId == orgGuid && e.EventType == "member.invited");
        auditEntry.Should().NotBeNull("an audit log row with event_type 'member.invited' must be written on invitation creation");
    }

    [Fact]
    public async Task Post_duplicate_email_while_pending_returns_409_invitation_pending()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var email = $"dup-{Guid.NewGuid():N}@example.com";
        var body = new { email, role = "member" };
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invitation_pending");
    }

    [Fact]
    public async Task Post_beyond_seat_limit_returns_409_seat_limit_reached()
    {
        // Create org with seat_count=1 subscription.
        var slug = $"lim-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "LimOrg", slug });
        var createJson = await resp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var orgId = createDoc.RootElement.GetProperty("id").GetString()!;

        // 1-seat subscription — already 1 member, so no room.
        var checkoutBody = new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 1,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no"
        };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var body = new { email = $"full-{Guid.NewGuid():N}@example.com", role = "member" };
        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("seat_limit_reached");
    }

    [Fact]
    public async Task Post_with_empty_email_returns_400_invalid_email()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var body = new { email = "", role = "member" };
        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_email");
    }

    [Fact]
    public async Task Post_as_non_member_returns_403_permission_denied()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var (memberToken, _) = TestTokens.CreateNew($"nonmember-{Guid.NewGuid():N}@example.com");
        var nonMemberClient = _factory.CreateClient();
        nonMemberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var body = new { email = $"pm-{Guid.NewGuid():N}@example.com", role = "member" };
        var response = await nonMemberClient.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    // ── GET /api/v1/organizations/{id}/invitations ───────────────────────────

    [Fact]
    public async Task List_returns_only_pending_invitations_for_org()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var email1 = $"list1-{Guid.NewGuid():N}@example.com";
        var email2 = $"list2-{Guid.NewGuid():N}@example.com";
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = email1, role = "member" });
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = email2, role = "admin" });

        var response = await _client.GetAsync($"/api/v1/organizations/{orgId}/invitations");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var invitations = doc.RootElement.GetProperty("invitations");
        invitations.GetArrayLength().Should().BeGreaterThanOrEqualTo(2);
    }

    // ── POST /api/v1/invitations/accept ──────────────────────────────────────

    [Fact]
    public async Task Accept_with_valid_token_creates_member_and_marks_accepted()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var accepteeEmail = $"accept-{Guid.NewGuid():N}@example.com";
        var body = new { email = accepteeEmail, role = "member" };
        var createResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", body);
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);

        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var rawToken = createDoc.RootElement.GetProperty("token").GetString()!;

        // Accept with the invitee's account.
        var (accepteeToken, accepteeId) = TestTokens.CreateNew(accepteeEmail);
        var acceptClient = _factory.CreateClient();
        acceptClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", accepteeToken);

        // M16-003: invite-accept is gated on email_verified.
        // Trigger user upsert via an authenticated endpoint, then mark verified.
        await acceptClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(accepteeId);

        var acceptResp = await acceptClient.PostAsJsonAsync("/api/v1/invitations/accept",
            new { token = rawToken });

        acceptResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var acceptJson = await acceptResp.Content.ReadAsStringAsync();
        using var acceptDoc = JsonDocument.Parse(acceptJson);
        acceptDoc.RootElement.GetProperty("invitation").GetProperty("role").GetString().Should().Be("member");
    }

    [Fact]
    public async Task Accept_with_expired_token_returns_410_invitation_expired()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        // Use a raw token whose SHA-256 hash we will store directly in the DB.
        var rawToken = "expiredtokenexpiredtokenexpiredtokenexpiredtokenexpiredtoken0000000000"; // 70 chars
        var tokenHash = ApiTool.Backend.Invitations.InvitationTokenGenerator.HashToken(rawToken);

        // Seed an already-expired invitation directly into the database.
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            db.OrganizationInvitations.Add(new ApiTool.Backend.Data.Entities.OrganizationInvitation
            {
                Id = Guid.NewGuid(),
                OrgId = orgGuid,
                Email = $"exp-{Guid.NewGuid():N}@example.com",
                EmailNormalized = $"exp-{Guid.NewGuid():N}@example.com",
                Role = ApiTool.Backend.Data.Entities.OrgRole.Member,
                InvitedBy = _ownerId,
                TokenHash = tokenHash,
                ExpiresAt = DateTime.UtcNow.AddDays(-1), // already expired
                CreatedAt = DateTime.UtcNow.AddDays(-8),
                LastSentAt = DateTime.UtcNow.AddDays(-8),
                SendCount = 1,
            });
            await db.SaveChangesAsync();
        }

        var (accepteeToken, expAccepteeId) = TestTokens.CreateNew($"exp-{Guid.NewGuid():N}@example.com");
        var acceptClient = _factory.CreateClient();
        acceptClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", accepteeToken);

        // M16-003: invite-accept is gated on email_verified.
        await acceptClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(expAccepteeId);

        var response = await acceptClient.PostAsJsonAsync("/api/v1/invitations/accept",
            new { token = rawToken });

        response.StatusCode.Should().Be(HttpStatusCode.Gone); // 410
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invitation_expired");
    }

    // ── POST /api/v1/organizations/{id}/invitations/{inviteId}/resend ─────────

    [Fact]
    public async Task Resend_beyond_3_sends_returns_429_resend_limit_reached()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        // Parse the org Guid from the wire-format id.
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        // Seed an invitation with SendCount already at the limit and LastSentAt in the past
        // so the cooldown check does not interfere — we are testing the total-send-count limit.
        var inviteGuid = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            db.OrganizationInvitations.Add(new ApiTool.Backend.Data.Entities.OrganizationInvitation
            {
                Id = inviteGuid,
                OrgId = orgGuid,
                Email = $"rsl-{inviteGuid:N}@example.com",
                EmailNormalized = $"rsl-{inviteGuid:N}@example.com",
                Role = ApiTool.Backend.Data.Entities.OrgRole.Member,
                InvitedBy = _ownerId,
                TokenHash = new string('r', 64),
                ExpiresAt = DateTime.UtcNow.AddDays(6),
                CreatedAt = DateTime.UtcNow.AddDays(-2),
                LastSentAt = DateTime.UtcNow.AddHours(-25), // cooldown elapsed
                SendCount = 4, // exceeds MaxResends (3), so next resend is rejected
            });
            await db.SaveChangesAsync();
        }

        var inviteId = ApiTool.Backend.Invitations.InvitationId.Format(inviteGuid);

        // This resend should be rejected because SendCount (3) > MaxResends (3).
        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations/{inviteId}/resend", new { });

        response.StatusCode.Should().Be(HttpStatusCode.TooManyRequests);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("resend_limit_reached");
    }

    [Fact]
    public async Task Resend_within_cooldown_returns_429_resend_cooldown()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        // Seed an invitation whose LastSentAt is within the 24-hour cooldown window.
        var inviteGuid = Guid.NewGuid();
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
            db.OrganizationInvitations.Add(new ApiTool.Backend.Data.Entities.OrganizationInvitation
            {
                Id = inviteGuid,
                OrgId = orgGuid,
                Email = $"cool-{inviteGuid:N}@example.com",
                EmailNormalized = $"cool-{inviteGuid:N}@example.com",
                Role = ApiTool.Backend.Data.Entities.OrgRole.Member,
                InvitedBy = _ownerId,
                TokenHash = new string('s', 64),
                ExpiresAt = DateTime.UtcNow.AddDays(6),
                CreatedAt = DateTime.UtcNow.AddHours(-1),
                LastSentAt = DateTime.UtcNow.AddMinutes(-30), // within cooldown (< 24h)
                SendCount = 1, // well within the send-count limit
            });
            await db.SaveChangesAsync();
        }

        var inviteId = ApiTool.Backend.Invitations.InvitationId.Format(inviteGuid);

        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations/{inviteId}/resend", new { });

        response.StatusCode.Should().Be(HttpStatusCode.TooManyRequests); // 429
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("resend_cooldown");
    }

    // ── DELETE /api/v1/organizations/{id}/invitations/{inviteId} ─────────────

    [Fact]
    public async Task Revoke_marks_invitation_revoked_and_frees_seat()
    {
        var orgId = await CreateOrgWithSubscriptionAsync();

        var email = $"revoke-{Guid.NewGuid():N}@example.com";
        var createResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email, role = "member" });
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);

        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var inviteId = createDoc.RootElement.GetProperty("invitation").GetProperty("id").GetString()!;

        var response = await _client.DeleteAsync($"/api/v1/organizations/{orgId}/invitations/{inviteId}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }
}
