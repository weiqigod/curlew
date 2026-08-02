using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Rbac;

/// <summary>Full behaviour-level integration tests for the custom roles endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class CustomRolesEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _ownerClient;
    private readonly Guid _ownerId;

    public CustomRolesEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"roles-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _ownerClient = factory.CreateClient();
        _ownerClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        // M16-003: checkout is gated on email_verified.
        await _ownerClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(_ownerId);
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private async Task<string> CreateOrgAsync()
    {
        var slug = $"rol-{Guid.NewGuid():N}"[..20];
        var resp = await _ownerClient.PostAsJsonAsync("/api/v1/organizations", new { name = "RolesOrg", slug });
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.GetProperty("id").GetString()!;
    }

    private async Task<(HttpClient client, Guid userId)> InviteAndAcceptAsync(
        string orgId, string role = "member")
    {
        // Give org a subscription so invitations work.
        await _ownerClient.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            org_id = orgId,
            tier = "team",
            interval = "month",
            seat_count = 5,
            success_url = "http://localhost/ok",
            cancel_url = "http://localhost/no",
        });

        var userId = Guid.NewGuid();
        var email = $"user-{userId:N}@example.com";

        var inviteResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations",
            new { email, role });
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        using var inviteDoc = JsonDocument.Parse(inviteJson);
        var rawToken = inviteDoc.RootElement.GetProperty("token").GetString()!;

        var userToken = TestTokens.Create(userId, email);
        var userClient = _factory.CreateClient();
        userClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", userToken);

        // M16-003: invite-accept is gated on email_verified. Upsert the acceptee via any
        // authenticated request then mark them verified before accepting.
        await userClient.GetAsync("/api/v1/subscriptions");
        await _factory.SetEmailVerifiedAsync(userId);

        await userClient.PostAsJsonAsync("/api/v1/invitations/accept", new { token = rawToken });

        return (userClient, userId);
    }

    private async Task<string> GetMemberIdAsync(string orgId, string role)
    {
        var resp = await _ownerClient.GetAsync($"/api/v1/organizations/{orgId}/members");
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.GetProperty("members")
            .EnumerateArray()
            .First(m => m.GetProperty("role").GetString() == role)
            .GetProperty("user_id").GetString()!;
    }

    // ── Test 1: POST happy path ───────────────────────────────────────────────

    [Fact]
    public async Task Post_creates_custom_role_and_returns_201_with_role_id()
    {
        var orgId = await CreateOrgAsync();

        var resp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "results-only", permissions = new[] { "results.view", "results.upload" } });

        resp.StatusCode.Should().Be(HttpStatusCode.Created);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("id").GetString().Should().StartWith("role_");
        doc.RootElement.GetProperty("name").GetString().Should().Be("results-only");
        doc.RootElement.GetProperty("is_builtin").GetBoolean().Should().BeFalse();
    }

    // ── Test 2: POST + GET list includes built-ins + custom ──────────────────

    [Fact]
    public async Task Post_then_Get_list_includes_builtins_and_custom()
    {
        var orgId = await CreateOrgAsync();

        await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "my-role", permissions = new[] { "dashboard.view" } });

        var resp = await _ownerClient.GetAsync($"/api/v1/organizations/{orgId}/roles");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var roles = doc.RootElement.GetProperty("roles").EnumerateArray().ToList();
        roles.Count.Should().Be(4);

        var builtins = roles.Where(r => r.GetProperty("is_builtin").GetBoolean()).ToList();
        builtins.Count.Should().Be(3);
        builtins.Select(r => r.GetProperty("name").GetString()).Should().Contain("owner");
        builtins.Select(r => r.GetProperty("name").GetString()).Should().Contain("admin");
        builtins.Select(r => r.GetProperty("name").GetString()).Should().Contain("member");

        roles.Any(r => r.GetProperty("name").GetString() == "my-role").Should().BeTrue();
    }

    // ── Test 3: POST duplicate name → 409 ────────────────────────────────────

    [Fact]
    public async Task Post_duplicate_name_returns_409_role_name_taken()
    {
        var orgId = await CreateOrgAsync();

        await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "dup-role", permissions = new[] { "results.view" } });

        var resp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "dup-role", permissions = new[] { "dashboard.view" } });

        resp.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("role_name_taken");
    }

    // ── Test 4: POST unknown permission key → 400 ────────────────────────────

    [Fact]
    public async Task Post_unknown_permission_key_returns_400_invalid_permission_with_field()
    {
        var orgId = await CreateOrgAsync();

        var resp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "bad-perm", permissions = new[] { "results.view", "no.such.perm" } });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("invalid_permission");
        json.Should().Contain("no.such.perm");
    }

    // ── Test 5: POST empty name → 400 ────────────────────────────────────────

    [Fact]
    public async Task Post_empty_name_returns_400_invalid_name()
    {
        var orgId = await CreateOrgAsync();

        var resp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "   ", permissions = new[] { "results.view" } });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("invalid_name");
    }

    // ── Test 6: POST built-in name → 400 ─────────────────────────────────────

    [Theory]
    [InlineData("owner")]
    [InlineData("admin")]
    [InlineData("member")]
    public async Task Post_builtin_name_returns_400_invalid_name(string name)
    {
        var orgId = await CreateOrgAsync();

        var resp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name, permissions = new[] { "results.view" } });

        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("invalid_name");
    }

    // ── Test 7: POST by admin → 403 ──────────────────────────────────────────

    [Fact]
    public async Task Post_by_admin_returns_403_permission_denied()
    {
        var orgId = await CreateOrgAsync();
        var (adminClient, _) = await InviteAndAcceptAsync(orgId, "admin");

        var resp = await adminClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "admin-role", permissions = new[] { "results.view" } });

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("permission_denied");
    }

    // ── Test 8: POST by non-member → 404 ─────────────────────────────────────

    [Fact]
    public async Task Post_by_non_member_returns_404_organization_not_found()
    {
        var orgId = await CreateOrgAsync();

        var (outsiderToken, _) = TestTokens.CreateNew($"outside-{Guid.NewGuid():N}@example.com");
        var outsiderClient = _factory.CreateClient();
        outsiderClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", outsiderToken);

        var resp = await outsiderClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "some-role", permissions = new[] { "results.view" } });

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }

    // ── Test 9: GET by member succeeds ───────────────────────────────────────

    [Fact]
    public async Task Get_by_member_returns_200_with_builtin_roles()
    {
        var orgId = await CreateOrgAsync();
        var (memberClient, _) = await InviteAndAcceptAsync(orgId, "member");

        var resp = await memberClient.GetAsync($"/api/v1/organizations/{orgId}/roles");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var roles = doc.RootElement.GetProperty("roles").EnumerateArray().ToList();
        roles.Should().HaveCount(3); // three built-ins only
    }

    // ── Test 10: DELETE by owner when no members reference → 204 ─────────────

    [Fact]
    public async Task Delete_by_owner_with_no_members_referencing_returns_204()
    {
        var orgId = await CreateOrgAsync();

        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "delete-me", permissions = new[] { "results.view" } });
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        var resp = await _ownerClient.DeleteAsync($"/api/v1/organizations/{orgId}/roles/{roleId}");

        resp.StatusCode.Should().Be(HttpStatusCode.NoContent);
    }

    // ── Test 11: DELETE when members reference → 409 role_in_use ─────────────

    [Fact]
    public async Task Delete_when_members_reference_role_returns_409_role_in_use_with_count()
    {
        var orgId = await CreateOrgAsync();

        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "blocked-role", permissions = new[] { "results.view" } });
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        // Invite a member and assign the custom role.
        var (memberClient, memberId) = await InviteAndAcceptAsync(orgId, "member");
        var memberIdStr = await GetMemberIdAsync(orgId, "member");
        await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberIdStr}",
            new { role_id = roleId });

        var resp = await _ownerClient.DeleteAsync($"/api/v1/organizations/{orgId}/roles/{roleId}");

        resp.StatusCode.Should().Be(HttpStatusCode.Conflict);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("role_in_use");
        json.Should().Contain("member_count");
        _ = memberClient; // suppress unused variable warning
    }

    // ── Test 12: DELETE non-existent role → 404 ──────────────────────────────

    [Fact]
    public async Task Delete_non_existent_role_returns_404_role_not_found()
    {
        var orgId = await CreateOrgAsync();
        var fakeRoleId = $"role_{Guid.NewGuid():N}";

        var resp = await _ownerClient.DeleteAsync($"/api/v1/organizations/{orgId}/roles/{fakeRoleId}");

        resp.StatusCode.Should().Be(HttpStatusCode.NotFound);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("role_not_found");
    }

    // ── Test 13: DELETE by admin → 403 ───────────────────────────────────────

    [Fact]
    public async Task Delete_by_admin_returns_403_permission_denied()
    {
        var orgId = await CreateOrgAsync();

        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "admin-cannot-del", permissions = new[] { "results.view" } });
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        var (adminClient, _) = await InviteAndAcceptAsync(orgId, "admin");

        var resp = await adminClient.DeleteAsync($"/api/v1/organizations/{orgId}/roles/{roleId}");

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var json = await resp.Content.ReadAsStringAsync();
        json.Should().Contain("permission_denied");
    }

    // ── Test 14: Full flow - custom role assignment + permission check ─────────

    [Fact]
    public async Task Custom_role_member_passes_results_upload_gate_and_fails_members_invite_gate()
    {
        var orgId = await CreateOrgAsync();

        // Create a "results-only" custom role (no members.invite permission).
        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "results-only", permissions = new[] { "results.view", "results.upload", "dashboard.view" } });
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        // Invite a member and assign the custom role.
        var (memberClient, _) = await InviteAndAcceptAsync(orgId, "member");
        var memberIdStr = await GetMemberIdAsync(orgId, "member");

        var patchResp = await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberIdStr}",
            new { role_id = roleId });
        patchResp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Member with results.upload permission can upload results → 202.
        var resultPayload = new
        {
            collection_name = "smoke",
            run_at = "2026-04-19T12:00:00Z",
            duration_ms = 100,
            pass_count = 1,
            fail_count = 0,
            skipped_count = 0,
            items = new[] { new { name = "test-1", status = "passed", duration_ms = 50 } },
        };
        var uploadResp = await memberClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/results",
            resultPayload);
        uploadResp.StatusCode.Should().Be(HttpStatusCode.Accepted);

        // Member WITHOUT members.invite permission cannot invite others → 403 permission_denied.
        var inviteResp = await memberClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations",
            new { email = "newuser@example.com", role = "member" });
        inviteResp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var inviteJson = await inviteResp.Content.ReadAsStringAsync();
        inviteJson.Should().Contain("permission_denied");
    }

    // ── Test 14b: Custom role WITH members.invite can invite others ───────────

    [Fact]
    public async Task Custom_role_member_with_members_invite_permission_can_invite()
    {
        var orgId = await CreateOrgAsync();

        // Create a role that includes members.invite.
        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "inviter-role", permissions = new[] { "members.invite", "members.view", "results.view" } });
        createResp.StatusCode.Should().Be(HttpStatusCode.Created);
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        // Invite a member and assign the inviter role.
        var (memberClient, _) = await InviteAndAcceptAsync(orgId, "member");
        var memberIdStr = await GetMemberIdAsync(orgId, "member");

        var patchResp = await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberIdStr}",
            new { role_id = roleId });
        patchResp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Member WITH members.invite in their custom role can send invitations → 201.
        var inviteResp = await memberClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/invitations",
            new { email = "newuser-can-invite@example.com", role = "member" });
        inviteResp.StatusCode.Should().Be(HttpStatusCode.Created);
    }

    // ── Test 15: PATCH /members with role_id writes audit row ─────────────────

    [Fact]
    public async Task Patch_member_with_role_id_writes_role_changed_audit_event()
    {
        var orgId = await CreateOrgAsync();

        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "audit-role", permissions = new[] { "results.view" } });
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        var (_, _) = await InviteAndAcceptAsync(orgId, "member");
        var memberIdStr = await GetMemberIdAsync(orgId, "member");

        var patchResp = await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/members/{memberIdStr}",
            new { role_id = roleId });
        patchResp.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify audit event was recorded.
        var auditResp = await _ownerClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log");
        auditResp.StatusCode.Should().Be(HttpStatusCode.OK);
        var auditJson = await auditResp.Content.ReadAsStringAsync();
        auditJson.Should().Contain("role.changed");
    }

    [Fact]
    public async Task Patch_role_by_owner_replaces_name_and_permissions_and_writes_audit_event()
    {
        var orgId = await CreateOrgAsync();
        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "before-edit", permissions = new[] { "results.view" } });
        var createJson = await createResp.Content.ReadAsStringAsync();
        using var createDoc = JsonDocument.Parse(createJson);
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        var response = await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles/{roleId}",
            new { name = "after-edit", permissions = new[] { "dashboard.view", "results.upload" } });

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        doc.RootElement.GetProperty("name").GetString().Should().Be("after-edit");
        doc.RootElement.GetProperty("permissions").EnumerateArray()
            .Select(p => p.GetString()).Should().BeEquivalentTo("dashboard.view", "results.upload");

        var auditResponse = await _ownerClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log");
        (await auditResponse.Content.ReadAsStringAsync()).Should().Contain("role.updated");
    }

    [Fact]
    public async Task Patch_role_to_duplicate_name_returns_409()
    {
        var orgId = await CreateOrgAsync();
        await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "existing-name", permissions = new[] { "results.view" } });
        var createResp = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles",
            new { name = "rename-me", permissions = new[] { "dashboard.view" } });
        using var createDoc = JsonDocument.Parse(await createResp.Content.ReadAsStringAsync());
        var roleId = createDoc.RootElement.GetProperty("id").GetString()!;

        var response = await _ownerClient.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}/roles/{roleId}",
            new { name = "existing-name", permissions = new[] { "results.view" } });

        response.StatusCode.Should().Be(HttpStatusCode.Conflict);
        (await response.Content.ReadAsStringAsync()).Should().Contain("role_name_taken");
    }
}
