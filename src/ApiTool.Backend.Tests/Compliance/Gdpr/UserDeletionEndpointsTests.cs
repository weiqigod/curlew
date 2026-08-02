using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Integration tests for GDPR account-deletion endpoints:
/// POST /api/v1/users/me/deletion-requests,
/// POST /api/v1/users/me/deletion-requests/cancel,
/// GET  /api/v1/users/me/deletion-requests/status.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class UserDeletionEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    private const string ValidPassword = "Hunter2IsNotAPassword!";

    public UserDeletionEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
    }

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    /// <summary>Creates a new user with a password, returns (user, JWT, authenticated HttpClient).</summary>
    private async Task<(User user, string jwt, HttpClient client)> SeedUserWithPasswordAsync()
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var hasher = scope.ServiceProvider.GetRequiredService<PasswordHasher>();

        var userId = Guid.NewGuid();
        var email = $"del-{userId:N}@example.com";
        var user = new User
        {
            Id = userId,
            Email = email,
            CreatedAt = DateTime.UtcNow,
            PasswordHash = hasher.Hash(ValidPassword),
        };
        db.Users.Add(user);
        await db.SaveChangesAsync();

        var jwt = TestTokens.Create(userId, email);
        var client = _factory.CreateClient();
        client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", jwt);
        return (user, jwt, client);
    }

    /// <summary>Issues a drto_ token by calling POST /api/v1/auth/reauth.</summary>
    private async Task<string> MintReauthTokenAsync(HttpClient client)
    {
        var resp = await client.PostAsJsonAsync("/api/v1/auth/reauth", new { password = ValidPassword });
        resp.StatusCode.Should().Be(HttpStatusCode.OK, "reauth endpoint must succeed for test setup");
        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        return doc.RootElement.GetProperty("reauth_token").GetString()!;
    }

    private RecordingEmailQueue GetEmailQueue() =>
        _factory.Services.GetRequiredService<RecordingEmailQueue>();

    // ── POST /api/v1/users/me/deletion-requests ─────────────────────────────

    [Fact]
    public async Task POST_unauthenticated_returns_401()
    {
        using var anon = _factory.CreateClient();
        var response = await anon.PostAsync("/api/v1/users/me/deletion-requests", null);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task POST_without_reauth_token_returns_401_reauth_required()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("reauth_required");
    }

    [Fact]
    public async Task POST_with_expired_reauth_token_returns_401_reauth_expired()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();

        // Manually insert an already-expired token
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.DeletionReauthPrefix);
        db.DeletionReauthTokens.Add(new DeletionReauthToken
        {
            Id = Guid.NewGuid(),
            UserId = user.Id,
            TokenHash = hash,
            IssuedAt = DateTime.UtcNow.AddMinutes(-10),
            ExpiresAt = DateTime.UtcNow.AddMinutes(-5), // already expired
        });
        await db.SaveChangesAsync();

        client.DefaultRequestHeaders.Add("X-Reauth-Token", plaintext);
        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("reauth_expired");
    }

    [Fact]
    public async Task POST_with_consumed_reauth_token_returns_401_reauth_consumed()
    {
        // Use a fresh user with no pending deletion so that AlreadyPending does not
        // short-circuit before the reauth token validation path is reached.
        var (user, _, client) = await SeedUserWithPasswordAsync();

        // Insert an already-consumed reauth token directly into the DB.
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.DeletionReauthPrefix);
        db.DeletionReauthTokens.Add(new DeletionReauthToken
        {
            Id = Guid.NewGuid(),
            UserId = user.Id,
            TokenHash = hash,
            IssuedAt = DateTime.UtcNow.AddMinutes(-2),
            ExpiresAt = DateTime.UtcNow.AddMinutes(8),
            ConsumedAt = DateTime.UtcNow.AddMinutes(-1), // already consumed
        });
        await db.SaveChangesAsync();

        client.DefaultRequestHeaders.Add("X-Reauth-Token", plaintext);
        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("reauth_consumed");
    }

    [Fact]
    public async Task POST_with_fresh_token_returns_202_and_sets_pending_deletion_at()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Accepted);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.Users.FindAsync(user.Id);
        updated!.PendingDeletionAt.Should().NotBeNull(
            because: "POST deletion-requests must set pending_deletion_at");
    }

    [Fact]
    public async Task POST_response_carries_finalizes_at_30_days_in_future()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var finalizesAt = doc.RootElement.GetProperty("finalizes_at").GetDateTimeOffset();
        finalizesAt.Should().BeCloseTo(DateTimeOffset.UtcNow.AddDays(30), TimeSpan.FromMinutes(2));
    }

    [Fact]
    public async Task POST_response_carries_cancel_url_under_account_data()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var cancelUrl = doc.RootElement.GetProperty("cancel_url").GetString();
        cancelUrl.Should().Contain("cancel-deletion");
    }

    [Fact]
    public async Task POST_with_already_pending_deletion_returns_409_already_pending()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        // First request succeeds
        await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        // Issue a fresh reauth token for the second attempt
        var reauth2 = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Remove("X-Reauth-Token");
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth2);

        var second = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        second.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await second.Content.ReadAsStringAsync();
        body.Should().Contain("already_pending");
    }

    [Fact]
    public async Task POST_enqueues_account_deletion_initiated_email_with_cancel_url_variable()
    {
        var queue = GetEmailQueue();
        queue.Clear();

        var (user, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        var emails = queue.Messages
            .Where(m => m.To == user.Email && m.TemplateSlug == "account_deletion_initiated")
            .ToList();
        emails.Should().ContainSingle(because: "deletion initiation must enqueue an email");
        emails[0].Variables.Should().ContainKey("cancel_url");
    }

    // ── POST /api/v1/users/me/deletion-requests/cancel ──────────────────────

    [Fact]
    public async Task POST_cancel_clears_pending_deletion_at_and_emits_audit_event()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        // Initiate deletion
        await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        // Cancel it
        var cancel = await client.PostAsync("/api/v1/users/me/deletion-requests/cancel", null);
        cancel.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var updated = await db.Users.FindAsync(user.Id);
        updated!.PendingDeletionAt.Should().BeNull(because: "cancel must clear pending_deletion_at");

        // Audit event
        var auditEvents = db.OrganizationAuditLog
            .Where(e => e.ActorId == user.Id && e.EventType == "account.deletion_cancelled")
            .ToList();
        auditEvents.Should().ContainSingle(because: "cancel must emit an audit event");
    }

    [Fact]
    public async Task POST_cancel_with_no_pending_request_returns_404_no_pending_request()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests/cancel", null);

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("no_pending_request");
    }

    // ── GET /api/v1/users/me/deletion-requests/status ───────────────────────

    [Fact]
    public async Task GET_status_with_no_pending_returns_404()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();

        var response = await client.GetAsync("/api/v1/users/me/deletion-requests/status");

        response.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    [Fact]
    public async Task GET_status_with_pending_returns_200_with_finalizes_at()
    {
        var (_, _, client) = await SeedUserWithPasswordAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        // Initiate deletion first
        await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        var status = await client.GetAsync("/api/v1/users/me/deletion-requests/status");
        status.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await status.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.TryGetProperty("finalizes_at", out _).Should().BeTrue();
        doc.RootElement.TryGetProperty("pending_deletion_at", out _).Should().BeTrue();
    }

    // ── Last-admin protection (Step 3 / M18-006 v4-7) ───────────────────────

    /// <summary>Seeds a user who is the sole owner of an org that has other members.</summary>
    private async Task<(User user, string jwt, HttpClient client, Guid orgId)> SeedUserWithBlockingOrgAsync()
    {
        var (user, jwt, client) = await SeedUserWithPasswordAsync();

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        var orgId = Guid.NewGuid();
        var otherMemberId = Guid.NewGuid();
        db.Users.Add(new User { Id = otherMemberId, Email = $"member-{otherMemberId:N}@x.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Block Org", Slug = $"block{orgId:N}"[..20],
            OwnerId = user.Id, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember { OrgId = orgId, UserId = user.Id, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow });
        db.OrganizationMembers.Add(new OrganizationMember { OrgId = orgId, UserId = otherMemberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();

        return (user, jwt, client, orgId);
    }

    [Fact]
    public async Task POST_with_blocking_org_returns_409_with_owner_cannot_leave_body()
    {
        var (_, _, client, _) = await SeedUserWithBlockingOrgAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("owner_cannot_leave",
            because: "blocking org check must return 409 with owner_cannot_leave code");
        body.Should().Contain("blocking_orgs",
            because: "response must include the blocking_orgs list");
    }

    [Fact]
    public async Task POST_with_blocking_org_does_not_consume_reauth_token()
    {
        var (user, _, client, _) = await SeedUserWithBlockingOrgAsync();
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        // First call — blocked by 409
        await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        // Remove the blocking member so the second call can proceed
        using var fixScope = _factory.Services.CreateScope();
        var fixDb = fixScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var members = fixDb.OrganizationMembers.Where(m => m.UserId != user.Id);
        fixDb.OrganizationMembers.RemoveRange(members);
        await fixDb.SaveChangesAsync();

        // Second call with the SAME reauth token — must succeed if token was not consumed
        client.DefaultRequestHeaders.Remove("X-Reauth-Token");
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);
        var second = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        second.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "the reauth token must not be consumed when the 409 is returned");
    }

    [Fact]
    public async Task POST_with_cascade_org_marks_org_pending_deletion()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();

        // Seed an org where user is sole owner, no other members
        using var seedScope = _factory.Services.CreateScope();
        var seedDb = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var orgId = Guid.NewGuid();
        seedDb.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Cascade Org", Slug = $"casc{orgId:N}"[..20],
            OwnerId = user.Id, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        seedDb.OrganizationMembers.Add(new OrganizationMember { OrgId = orgId, UserId = user.Id, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow });
        await seedDb.SaveChangesAsync();

        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "sole-owner-with-no-members should cascade (proceed with deletion)");

        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var org = await verifyDb.Organizations.FindAsync(orgId);
        org!.Status.Should().Be(OrgStatus.PendingDeletion,
            because: "cascade org must be marked PendingDeletion when user requests deletion");
    }

    [Fact]
    public async Task POST_owner_with_another_owner_proceeds_normally()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();

        // Seed an org where user is owner but another owner also exists
        using var seedScope = _factory.Services.CreateScope();
        var seedDb = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var otherOwnerId = Guid.NewGuid();
        seedDb.Users.Add(new User { Id = otherOwnerId, Email = $"other-owner-{otherOwnerId:N}@x.com", CreatedAt = DateTime.UtcNow });
        var orgId = Guid.NewGuid();
        seedDb.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Dual Owner Org", Slug = $"dual{orgId:N}"[..20],
            OwnerId = user.Id, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        seedDb.OrganizationMembers.Add(new OrganizationMember { OrgId = orgId, UserId = user.Id, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow });
        seedDb.OrganizationMembers.Add(new OrganizationMember { OrgId = orgId, UserId = otherOwnerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow });
        await seedDb.SaveChangesAsync();

        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);

        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Accepted,
            because: "user with another owner in the org should not be blocked");
    }

    /// <summary>
    /// Regression test for review finding #2 (iteration 2).
    /// A user who is already-pending AND the sole owner of a blocking org must receive
    /// already_pending (not owner_cannot_leave). AlreadyPending fires BEFORE the
    /// last-admin check.
    /// </summary>
    [Fact]
    public async Task POST_already_pending_and_blocking_returns_already_pending_not_owner_cannot_leave()
    {
        var (user, _, client, _) = await SeedUserWithBlockingOrgAsync();

        // First request — succeeds (user has no blocking org at this point? No, user IS blocking.
        // We need a user who is ALREADY pending. Set pending_deletion_at directly.)
        using var seedScope = _factory.Services.CreateScope();
        var seedDb = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var dbUser = await seedDb.Users.FindAsync(user.Id);
        dbUser!.PendingDeletionAt = DateTime.UtcNow;
        await seedDb.SaveChangesAsync();

        // Now POST — user is both already-pending AND blocking
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);
        var response = await client.PostAsync("/api/v1/users/me/deletion-requests", null);

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("already_pending",
            because: "AlreadyPending check must fire before the last-admin check");
        body.Should().NotContain("owner_cannot_leave",
            because: "the user already has a pending deletion — last-admin check must not run");
    }

    /// <summary>
    /// Regression test for review finding #2 (cascade unwind).
    /// When a user cancels their deletion request, any cascade orgs that were atomically
    /// marked PendingDeletion when the deletion was initiated must be reversed to Active.
    /// </summary>
    [Fact]
    public async Task POST_cancel_with_cascade_orgs_unwinds_them()
    {
        var (user, _, client) = await SeedUserWithPasswordAsync();

        // Seed a cascade org: user is sole owner, zero other members
        using var seedScope = _factory.Services.CreateScope();
        var seedDb = seedScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var orgId = Guid.NewGuid();
        seedDb.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Cascade Unwind Org", Slug = $"unwi{orgId:N}"[..20],
            OwnerId = user.Id, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        seedDb.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = user.Id, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        await seedDb.SaveChangesAsync();

        // Initiate deletion — this should mark the cascade org as PendingDeletion
        var reauth = await MintReauthTokenAsync(client);
        client.DefaultRequestHeaders.Add("X-Reauth-Token", reauth);
        var post = await client.PostAsync("/api/v1/users/me/deletion-requests", null);
        post.StatusCode.Should().Be(HttpStatusCode.Accepted, "setup: deletion initiation must succeed");

        // Verify org is now PendingDeletion
        using var midScope = _factory.Services.CreateScope();
        var midDb = midScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var orgMid = await midDb.Organizations.FindAsync(orgId);
        orgMid!.Status.Should().Be(OrgStatus.PendingDeletion, "setup: org should be PendingDeletion after initiation");

        // Cancel the deletion
        var cancel = await client.PostAsync("/api/v1/users/me/deletion-requests/cancel", null);
        cancel.StatusCode.Should().Be(HttpStatusCode.OK, "cancel must succeed");

        // Verify cascade org is now Active again
        using var verifyScope = _factory.Services.CreateScope();
        var verifyDb = verifyScope.ServiceProvider.GetRequiredService<AppDbContext>();
        var org = await verifyDb.Organizations.FindAsync(orgId);
        org!.Status.Should().Be(OrgStatus.Active,
            because: "cancelling deletion must revert cascade orgs from PendingDeletion to Active");
    }
}
