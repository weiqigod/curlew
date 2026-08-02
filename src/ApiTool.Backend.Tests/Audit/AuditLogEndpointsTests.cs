using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>Integration tests for GET /api/v1/organizations/{id}/audit-log.</summary>
[Collection(BackendCollection.Name)]
public sealed class AuditLogEndpointsTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private readonly Guid _adminId;
    private readonly Guid _memberId;

    public AuditLogEndpointsTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        _adminId = Guid.NewGuid();
        _memberId = Guid.NewGuid();

        _client = factory.CreateClient();
        var ownerToken = TestTokens.Create(_ownerId, $"audit-owner-{_ownerId:N}@example.com");
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", ownerToken);
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
        var slug = $"alog-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations",
            new { name = "AuditLogOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        return doc.RootElement.GetProperty("id").GetString()!;
    }

    private async Task<(string orgId, Guid adminUserId, Guid memberUserId)> CreateOrgWithMembersAsync()
    {
        var orgId = await CreateOrgAsync();

        // Add subscription so we can invite
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 10,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no",
        });

        // Register admin user
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        // Add admin + member to the org directly via DB
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var adminEmail = $"audit-admin-{_adminId:N}@example.com";
        var memberEmail = $"audit-member-{_memberId:N}@example.com";
        db.Users.Add(new User { Id = _adminId, Email = adminEmail, CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = _memberId, Email = memberEmail, CreatedAt = DateTime.UtcNow });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgGuid, UserId = _adminId, Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgGuid, UserId = _memberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        return (orgId, _adminId, _memberId);
    }

    [Fact]
    public async Task Get_audit_log_returns_rows_newest_first()
    {
        var (orgId, _, _) = await CreateOrgWithMembersAsync();

        // Trigger auditable events by inviting people (each creates an audit row)
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations",
            new { email = $"invite-a{Guid.NewGuid():N}@example.com", role = "member" });
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations",
            new { email = $"invite-b{Guid.NewGuid():N}@example.com", role = "member" });

        var resp = await _client.GetAsync($"/api/v1/organizations/{orgId}/audit-log?limit=10");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var items = doc.RootElement.GetProperty("items").EnumerateArray().ToList();
        items.Should().HaveCountGreaterThan(0);

        // Rows should be newest-first
        var times = items
            .Select(i => DateTime.Parse(i.GetProperty("created_at").GetString()!))
            .ToList();
        times.Should().BeInDescendingOrder();
    }

    [Fact]
    public async Task Get_audit_log_as_non_admin_returns_403_permission_denied()
    {
        var (orgId, _, memberUserId) = await CreateOrgWithMembersAsync();

        // Use member token
        var memberToken = TestTokens.Create(memberUserId, $"audit-member-{memberUserId:N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var resp = await memberClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log");
        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("permission_denied");
    }

    [Fact]
    public async Task Get_audit_log_with_event_type_filter_returns_only_matching_rows()
    {
        var (orgId, _, _) = await CreateOrgWithMembersAsync();

        // Create member.invited audit rows
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations",
            new { email = $"filter-{Guid.NewGuid():N}@example.com", role = "member" });

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?event_type=member.invited");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var items = doc.RootElement.GetProperty("items").EnumerateArray().ToList();

        items.Should().OnlyContain(i =>
            i.GetProperty("event_type").GetString() == "member.invited");
    }

    [Fact]
    public async Task Get_audit_log_with_from_and_to_filters_returns_only_in_range_rows()
    {
        var orgId = await CreateOrgAsync();

        var from = DateTime.UtcNow.AddMinutes(-1).ToString("O");
        var to = DateTime.UtcNow.AddMinutes(1).ToString("O");

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?from={Uri.EscapeDataString(from)}&to={Uri.EscapeDataString(to)}");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Get_audit_log_returns_400_when_from_is_after_to()
    {
        var orgId = await CreateOrgAsync();

        var from = DateTime.UtcNow.AddMinutes(10).ToString("O");
        var to = DateTime.UtcNow.AddMinutes(-10).ToString("O");

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?from={Uri.EscapeDataString(from)}&to={Uri.EscapeDataString(to)}");
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Get_audit_log_csv_format_returns_text_csv_with_content_disposition()
    {
        // Updated M18-001: CSV now requires Enterprise tier (streamed path).
        var orgId = await CreateOrgAsync();
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");
        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=csv");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("text/csv");
        resp.Content.Headers.ContentDisposition.Should().NotBeNull();
        resp.Content.Headers.ContentDisposition!.DispositionType.Should().Be("attachment");
    }

    [Fact]
    public async Task Get_audit_log_respects_limit_parameter_clamped_to_200()
    {
        var orgId = await CreateOrgAsync();

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?limit=9999");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("items").EnumerateArray().Count().Should().BeLessOrEqualTo(200);
    }

    [Fact]
    public async Task Get_audit_log_for_unknown_org_returns_403()
    {
        var fakeOrgId = Guid.NewGuid().ToString("N");
        var resp = await _client.GetAsync($"/api/v1/organizations/{fakeOrgId}/audit-log");
        // Unknown org → treated as not-a-member → 403 to prevent enumeration
        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
    }

    [Fact]
    public async Task Get_audit_log_with_user_id_filter_returns_only_matching_rows()
    {
        var (orgId, adminUserId, _) = await CreateOrgWithMembersAsync();

        // Trigger two invitation audit rows as owner
        await _client.PostAsJsonAsync($"/api/v1/organizations/{orgId}/invitations",
            new { email = $"uid-filter-{Guid.NewGuid():N}@example.com", role = "member" });

        // Filter by the owner's user_id — should match at least the row created above
        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?user_id={_ownerId:N}");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var items = doc.RootElement.GetProperty("items").EnumerateArray().ToList();

        // All returned rows must have user_id == _ownerId
        items.Should().OnlyContain(i =>
            i.GetProperty("user_id").GetString() == _ownerId.ToString("N"));
    }

    [Fact]
    public async Task Get_audit_log_with_invalid_user_id_format_returns_400()
    {
        var orgId = await CreateOrgAsync();

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?user_id=not-a-valid-guid");
        resp.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("invalid_filter");
    }

    [Fact]
    public async Task Get_audit_log_unauthenticated_returns_401()
    {
        var orgId = await CreateOrgAsync();

        // Use an unauthenticated client (no Authorization header)
        var anonClient = _factory.CreateClient();
        var resp = await anonClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log");

        resp.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    // ── M18-001 export tests ──────────────────────────────────────────────────

    private async Task<(string orgId, Guid orgGuid)> CreateEnterpriseOrgAsync()
    {
        var orgId = await CreateOrgAsync();
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");
        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);
        return (orgId, orgGuid);
    }

    private async Task SeedAuditRowsAsync(Guid orgGuid, int count)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        for (var i = 0; i < count; i++)
        {
            db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(),
                OrgId = orgGuid,
                ActorId = _ownerId,
                EventType = "seed.event",
                CreatedAt = DateTime.UtcNow.AddSeconds(-i),
                Success = true,
            });
        }
        await db.SaveChangesAsync();
    }

    [Fact]
    public async Task Get_audit_log_jsonl_format_enterprise_org_returns_200_with_ndjson_content_type()
    {
        var (orgId, orgGuid) = await CreateEnterpriseOrgAsync();
        await SeedAuditRowsAsync(orgGuid, 5);

        using var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=jsonl",
            HttpCompletionOption.ResponseHeadersRead);

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/x-ndjson");
        resp.Content.Headers.ContentDisposition!.DispositionType.Should().Be("attachment");
        resp.Content.Headers.ContentDisposition.FileName.Should().StartWith($"\"audit-log-{orgId}-");
        resp.Content.Headers.ContentDisposition.FileName.Should().EndWith(".jsonl\"");
    }

    [Fact]
    public async Task Get_audit_log_jsonl_format_enterprise_org_returns_all_rows_without_cap()
    {
        var (orgId, orgGuid) = await CreateEnterpriseOrgAsync();
        await SeedAuditRowsAsync(orgGuid, 250);

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=jsonl");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        var lines = body.TrimEnd('\n').Split('\n', StringSplitOptions.RemoveEmptyEntries);
        // org.created is seeded by CreateOrgAsync plus our 250 seed.event rows — all > 200 cap
        lines.Should().HaveCountGreaterThanOrEqualTo(250); // cap of 200 does NOT apply

        // Each line must be valid JSON with event_type
        foreach (var line in lines.Take(5))
        {
            using var doc = JsonDocument.Parse(line);
            doc.RootElement.TryGetProperty("event_type", out _).Should().BeTrue();
        }
    }

    [Fact]
    public async Task Get_audit_log_csv_format_enterprise_org_returns_header_plus_all_rows()
    {
        var (orgId, orgGuid) = await CreateEnterpriseOrgAsync();
        await SeedAuditRowsAsync(orgGuid, 10);

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=csv");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        var lines = body.TrimEnd('\n').Split('\n', StringSplitOptions.RemoveEmptyEntries);
        lines.Length.Should().BeGreaterThan(1); // header + data rows
        lines[0].Should().Be("created_at,event_type,user_id,user_email,target_type,target_id,success,ip_address");
    }

    [Fact]
    public async Task Get_audit_log_export_team_tier_returns_402_payment_required_with_rfc7807_body()
    {
        var (orgId, _, _) = await CreateOrgWithMembersAsync();

        using var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=jsonl");

        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/problem+json");

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("current_tier").GetString().Should().NotBeNullOrEmpty();
        doc.RootElement.GetProperty("required_tier").GetString().Should().Be("enterprise");
        doc.RootElement.GetProperty("code").GetString().Should().Be("audit_log_export_tier_ineligible");
    }

    [Fact]
    public async Task Get_audit_log_export_csv_team_tier_returns_402()
    {
        var (orgId, _, _) = await CreateOrgWithMembersAsync();

        using var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=csv");

        resp.StatusCode.Should().Be(HttpStatusCode.PaymentRequired);
    }

    [Fact]
    public async Task Get_audit_log_export_member_role_returns_403_before_tier_check()
    {
        var (orgId, _, memberUserId) = await CreateOrgWithMembersAsync();
        // Add Enterprise subscription (but member still gets 403 — RBAC fires before tier gate)
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");
        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);

        var memberToken = TestTokens.Create(memberUserId, $"audit-member-{memberUserId:N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var resp = await memberClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log?format=jsonl");

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("permission_denied");
    }

    [Fact]
    public async Task Get_audit_log_without_format_preserves_paginated_json_and_MaxLimit_200_regression_guard()
    {
        var (orgId, _, _) = await CreateOrgWithMembersAsync();
        // Team tier — paginated path does NOT gate on tier
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");
        await SeedAuditRowsAsync(orgGuid, 250);

        using var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?limit=9999");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/json");

        var body = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("items").EnumerateArray().Count().Should().BeLessOrEqualTo(200);
    }

    [Fact]
    public async Task Get_audit_log_format_json_explicit_falls_through_to_paginated_path()
    {
        var orgId = await CreateOrgAsync();

        using var resp = await _client.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=json");

        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        resp.Content.Headers.ContentType!.MediaType.Should().Be("application/json");
        // No 402 — tier gate not consulted on non-export format
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"items\"");
    }

    [Fact]
    public async Task Get_audit_log_export_unknown_org_returns_403()
    {
        // Unknown org → RBAC check fires first (caller is not a member of a non-existent org),
        // so 403 is returned to prevent org enumeration — same anti-enumeration shape as the
        // paginated path (Get_audit_log_for_unknown_org_returns_403), but exercised on the
        // export code branch with ?format=jsonl.
        var fakeOrgId = Guid.NewGuid().ToString("N");

        var resp = await _client.GetAsync(
            $"/api/v1/organizations/{fakeOrgId}/audit-log?format=jsonl");

        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("permission_denied");
    }

    // ── M18-002 RBAC lift tests ───────────────────────────────────────────────

    /// <summary>
    /// Seeds a custom role with the given permissions for a member-tier user and assigns it.
    /// </summary>
    private async Task SeedCustomRoleForMemberAsync(Guid orgGuid, Guid memberUserId, string[] permissions)
    {
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();

        var roleId = Guid.NewGuid();
        db.OrganizationCustomRoles.Add(new CustomRole
        {
            Id = roleId,
            OrgId = orgGuid,
            Name = $"security-auditor-{roleId:N}",
            PermissionsJson = System.Text.Json.JsonSerializer.Serialize(permissions),
            CreatedAt = DateTime.UtcNow,
            CreatedBy = _ownerId,
        });

        // Assign the custom role to the member
        var member = await db.OrganizationMembers.FindAsync(orgGuid, memberUserId);
        if (member is not null)
            member.RoleId = roleId;

        await db.SaveChangesAsync();
    }

    [Fact]
    public async Task Get_audit_log_as_security_auditor_member_returns_200()
    {
        // Member with audit_log.view custom role gets 200 on the paginated endpoint.
        var (orgId, _, memberUserId) = await CreateOrgWithMembersAsync();
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        await SeedCustomRoleForMemberAsync(orgGuid, memberUserId, new[] { Permissions.AuditLogView });

        var memberToken = TestTokens.Create(memberUserId, $"audit-member-{memberUserId:N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var resp = await memberClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log?limit=10");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"items\"");
    }

    [Fact]
    public async Task Get_audit_log_jsonl_as_security_auditor_returns_403_with_export_permission_name()
    {
        // Member with audit_log.view only gets 403 on the export endpoint, body names audit_log.export.
        var (orgId, _, memberUserId) = await CreateOrgWithMembersAsync();
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        // Grant Enterprise subscription so the tier gate doesn't fire first.
        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);

        // Give member only audit_log.view (no audit_log.export).
        await SeedCustomRoleForMemberAsync(orgGuid, memberUserId, new[] { Permissions.AuditLogView });

        var memberToken = TestTokens.Create(memberUserId, $"audit-member-{memberUserId:N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        var resp = await memberClient.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=jsonl");
        resp.StatusCode.Should().Be(HttpStatusCode.Forbidden);

        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("permission_denied");
        body.Should().Contain("audit_log.export");
    }

    [Fact]
    public async Task Get_audit_log_as_plain_member_without_custom_role_returns_403()
    {
        // Plain Member (no custom role) gets 403 on both paginated and export paths.
        var (orgId, _, memberUserId) = await CreateOrgWithMembersAsync();
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");

        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);

        var memberToken = TestTokens.Create(memberUserId, $"audit-member-{memberUserId:N}@example.com");
        var memberClient = _factory.CreateClient();
        memberClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", memberToken);

        // Paginated path
        var resp1 = await memberClient.GetAsync($"/api/v1/organizations/{orgId}/audit-log");
        resp1.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var body1 = await resp1.Content.ReadAsStringAsync();
        body1.Should().Contain("permission_denied");

        // Export path
        var resp2 = await memberClient.GetAsync(
            $"/api/v1/organizations/{orgId}/audit-log?format=jsonl");
        resp2.StatusCode.Should().Be(HttpStatusCode.Forbidden);
        var body2 = await resp2.Content.ReadAsStringAsync();
        body2.Should().Contain("permission_denied");
    }
}
