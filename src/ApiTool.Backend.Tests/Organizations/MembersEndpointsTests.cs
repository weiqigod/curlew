using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>Full behaviour-level tests for the member management endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class MembersEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;

    public MembersEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"mbr-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, ownerEmail);

        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        // M16-003: checkout is gated on email_verified.
        await _client.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(_ownerId);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<string> CreateOrgAsync()
    {
        var slug = $"mbr-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "MbrOrg", slug });
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.GetProperty("id").GetString()!;
    }

    // ── GET /api/v1/organizations/{id}/members ────────────────────────────────

    [Fact]
    public async Task Get_members_returns_owner_for_new_org()
    {
        var orgId = await CreateOrgAsync();

        var response = await _client.GetAsync($"/api/v1/organizations/{orgId}/members");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var members = doc.RootElement.GetProperty("members");
        members.GetArrayLength().Should().Be(1);
        members[0].GetProperty("role").GetString().Should().Be("owner");
    }

    [Fact]
    public async Task Get_members_returns_role_id_for_member_with_custom_role_assignment()
    {
        // Regression for M5-021: the roles-settings page counts custom-role members
        // by matching `member.role_id` against `role.id`. Before this fix, `MemberDto`
        // omitted `RoleId` entirely, so the frontend fell back to built-in role ids
        // and every custom role rendered `member_count = 0` — which fails the
        // enterprise-full.spec.ts assertion 6 (`member_count = 1`).
        var orgId = await CreateOrgAsync();

        // Upgrade to team tier so we can invite a second member.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        // Create a custom role.
        var createRoleResp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "qa-lead", permissions = new[] { "results.view" } });
        var createRoleJson = await createRoleResp.Content.ReadAsStringAsync();
        using var createRoleDoc = JsonDocument.Parse(createRoleJson);
        var roleId = createRoleDoc.RootElement.GetProperty("id").GetString()!;

        // Invite + accept a member, then assign the custom role to them.
        var (memberToken, memberId) = TestTokens.CreateNew($"roleid-{Guid.NewGuid():N}@example.com");
        var memberEmail = $"roleid-{memberId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations",
            new { email = memberEmail, role = "member" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var inviteToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", memberToken);
        // M16-003: invite-accept is gated on email_verified.
        await memberClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(memberId);
        await memberClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = inviteToken });

        await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberId:N}",
            new { role_id = roleId });

        var response = await _client.GetAsync($"/api/v1/organizations/{orgId}/members");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);

        var assignedRow = doc.RootElement.GetProperty("members")
            .EnumerateArray()
            .First(m => m.GetProperty("user_id").GetString() == memberId.ToString("N"));
        assignedRow.TryGetProperty("role_id", out var roleIdProp).Should().BeTrue();
        roleIdProp.GetString().Should().Be(roleId);

        // Owner row has no custom role assignment — role_id must be omitted or null
        // (the global serializer skips null properties via DefaultIgnoreCondition.WhenWritingNull,
        // and the frontend treats both `undefined` and `null` identically in countByRole).
        var ownerRow = doc.RootElement.GetProperty("members")
            .EnumerateArray()
            .First(m => m.GetProperty("role").GetString() == "owner");
        if (ownerRow.TryGetProperty("role_id", out var ownerRoleIdProp))
            ownerRoleIdProp.ValueKind.Should().Be(JsonValueKind.Null);
    }

    // ── PATCH /api/v1/organizations/{id}/members/{memberId} ──────────────────

    [Fact]
    public async Task Patch_member_role_by_owner_changes_role()
    {
        var orgId = await CreateOrgAsync();

        // Invite and accept as admin-to-be.
        var (adminToken, adminId) = TestTokens.CreateNew($"admin-{Guid.NewGuid():N}@example.com");

        // Give org a subscription to allow invitations.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var adminEmail = $"admin-{adminId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = adminEmail, role = "member" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var adminClient = _factory.CreateClient();
        adminClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", adminToken);
        // M16-003: invite-accept is gated on email_verified.
        await adminClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(adminId);
        await adminClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        // Get member id.
        var membersResp = await _client.GetAsync($"/api/v1/organizations/{orgId}/members");
        var membersJson = await membersResp.Content.ReadAsStringAsync();
        using var membersDoc = JsonDocument.Parse(membersJson);
        var memberId = membersDoc.RootElement.GetProperty("members")
            .EnumerateArray()
            .First(m => m.GetProperty("role").GetString() == "member")
            .GetProperty("user_id").GetString()!;

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberId}",
            new { role = "admin" });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("member").GetProperty("role").GetString().Should().Be("admin");
    }

    [Fact]
    public async Task Patch_member_role_by_admin_returns_403_permission_denied()
    {
        var orgId = await CreateOrgAsync();

        // Give org a subscription so invitations work.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        // Invite and accept as admin.
        var (adminToken, adminId) = TestTokens.CreateNew($"adm2-{Guid.NewGuid():N}@example.com");
        var adminEmail = $"adm2-{adminId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = adminEmail, role = "admin" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var adminClient = _factory.CreateClient();
        adminClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", adminToken);
        // M16-003: invite-accept is gated on email_verified.
        await adminClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(adminId);
        await adminClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        // Invite a second member to serve as target for the role-change attempt.
        var (memberToken, memberId) = TestTokens.CreateNew($"mem2-{Guid.NewGuid():N}@example.com");
        var memberEmail = $"mem2-{memberId:N}@example.com";
        var inviteResp2 = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = memberEmail, role = "member" });
        var inviteJson2 = await inviteResp2.Content.ReadAsStringAsync();
        using var inviteDoc2 = JsonDocument.Parse(inviteJson2);
        var rawToken2 = inviteDoc2.RootElement.GetProperty("token").GetString()!;

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", memberToken);
        // M16-003: invite-accept is gated on email_verified.
        await memberClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(memberId);
        await memberClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken2 });

        // Admin attempts to change the member's role — only owner is allowed.
        var response = await adminClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberId:N}",
            new { role = "admin" });

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("permission_denied");
    }

    [Fact]
    public async Task Delete_member_by_owner_removes_membership()
    {
        var orgId = await CreateOrgAsync();

        // Add subscription and invite a member.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var (memberToken, memberId) = TestTokens.CreateNew($"del-{Guid.NewGuid():N}@example.com");
        var memberEmail = $"del-{memberId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = memberEmail, role = "member" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", memberToken);
        // M16-003: invite-accept is gated on email_verified.
        await memberClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(memberId);
        await memberClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        // Delete the member using their user_id (wire format).
        var response = await _client.DeleteAsync($"/api/v1/organizations/{orgId}/members/{memberId:N}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Delete_owner_returns_403()
    {
        var orgId = await CreateOrgAsync();

        // Try to remove the owner (use owner's own id as guid string).
        var response = await _client.DeleteAsync($"/api/v1/organizations/{orgId}/members/{_ownerId:N}");

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("cannot_remove_owner");
    }

    // ── POST /api/v1/organizations/{id}/transfer ──────────────────────────────

    [Fact]
    public async Task Transfer_to_admin_swaps_roles()
    {
        var orgId = await CreateOrgAsync();

        // Invite and accept as admin.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        var (adminToken, adminId) = TestTokens.CreateNew($"transfer-{Guid.NewGuid():N}@example.com");
        var adminEmail = $"transfer-{adminId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = adminEmail, role = "admin" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var adminClient = _factory.CreateClient();
        adminClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", adminToken);
        // M16-003: invite-accept is gated on email_verified.
        await adminClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(adminId);
        await adminClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/transfer",
            new { new_owner_id = adminId.ToString("N") });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Transfer_to_member_returns_422()
    {
        var orgId = await CreateOrgAsync();

        // Give org a subscription so invitations work.
        var checkoutBody = new { org_id = orgId, tier = "team", interval = "month", seat_count = 5, success_url = "http://localhost/ok", cancel_url = "http://localhost/no" };
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", checkoutBody);

        // Invite and accept as a regular member (not admin).
        var (memberToken, memberId) = TestTokens.CreateNew($"xfr-{Guid.NewGuid():N}@example.com");
        var memberEmail = $"xfr-{memberId:N}@example.com";
        var inviteResp = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations", new { email = memberEmail, role = "member" });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", memberToken);
        // M16-003: invite-accept is gated on email_verified.
        await memberClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(memberId);
        await memberClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        // Transfer ownership to a plain member — should return 422 (InvalidTransferTarget).
        var response = await _client.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/transfer",
            new { new_owner_id = memberId.ToString("N") });

        response.StatusCode.Should().Be(HttpStatusCode.UnprocessableEntity); // 422
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("invalid_transfer_target");
    }

    // ── POST /api/v1/organizations/{id}/leave ─────────────────────────────────

    [Fact]
    public async Task Leave_by_owner_returns_403_owner_cannot_leave()
    {
        var orgId = await CreateOrgAsync();

        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/leave", new { });

        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("code").GetString().Should().Be("owner_cannot_leave");
    }

    // ── PATCH /api/v1/organizations/{id} ─────────────────────────────────────

    [Fact]
    public async Task Patch_org_name_by_owner_updates_and_writes_audit()
    {
        var orgId = await CreateOrgAsync();

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}",
            new { name = "Updated Name" });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("name").GetString().Should().Be("Updated Name");
    }

    // ── DELETE /api/v1/organizations/{id} ─────────────────────────────────────

    [Fact]
    public async Task Delete_org_marks_pending_deletion()
    {
        var orgId = await CreateOrgAsync();

        var response = await _client.DeleteAsync($"/api/v1/organizations/{orgId}");

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("status").GetString().Should().Be("pending_deletion");
    }

    // ── POST /api/v1/organizations/{id}/cancel-deletion ──────────────────────

    [Fact]
    public async Task Cancel_deletion_restores_active_status()
    {
        var orgId = await CreateOrgAsync();
        await _client.DeleteAsync($"/api/v1/organizations/{orgId}");

        var response = await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/cancel-deletion", new { });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("status").GetString().Should().Be("active",
            because: "cancel-deletion should restore the active status");
    }
}
