using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>
/// Behaviour-level tests for the audit_log_retention_days field on the
/// PATCH /api/v1/organizations/{id} endpoint (M18-002).
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class MembersUpdateOrgRetentionTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;

    public MembersUpdateOrgRetentionTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var ownerEmail = $"ret-owner-{_ownerId:N}@example.com";
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

    private async Task<(string orgId, Guid orgGuid)> CreateOrgAsync()
    {
        var slug = $"ret-{Guid.NewGuid():N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "RetentionOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var orgId = doc.RootElement.GetProperty("id").GetString()!;
        if (!ApiTool.Backend.Organizations.OrgId.TryParse(orgId, out var orgGuid))
            throw new InvalidOperationException("Could not parse org id.");
        return (orgId, orgGuid);
    }

    [Fact]
    public async Task Update_org_sets_audit_log_retention_days_to_valid_value()
    {
        // Non-enterprise org (Team tier) can set retention_days <= 365.
        var (orgId, orgGuid) = await CreateOrgAsync();
        // Checkout to get a team subscription
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no",
        });

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}",
            new { audit_log_retention_days = 180 });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify persisted value
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var org = await db.Organizations.FindAsync(orgGuid);
        org!.AuditLogRetentionDays.Should().Be(180);
    }

    [Fact]
    public async Task Update_org_rejects_retention_days_above_365_for_non_enterprise()
    {
        // Non-Enterprise org cannot set retention_days > 365.
        var (orgId, _) = await CreateOrgAsync();
        await _client.PostAsJsonAsync("/api/v1/subscriptions/checkout", new
        {
            org_id = orgId, tier = "team", interval = "month", seat_count = 5,
            success_url = "http://localhost/ok", cancel_url = "http://localhost/no",
        });

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}",
            new { audit_log_retention_days = 400 });

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("retention_days_exceeds_cap");
    }

    [Fact]
    public async Task Update_org_allows_retention_days_above_365_for_enterprise()
    {
        // Enterprise org can set retention_days > 365.
        var (orgId, orgGuid) = await CreateOrgAsync();
        await _factory.SeedSubscriptionAsync(orgGuid, SubscriptionTier.Enterprise);

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}",
            new { audit_log_retention_days = 730 });

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        // Verify persisted value
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var org = await db.Organizations.FindAsync(orgGuid);
        org!.AuditLogRetentionDays.Should().Be(730);
    }

    [Fact]
    public async Task Update_org_rejects_non_positive_retention_days()
    {
        var (orgId, _) = await CreateOrgAsync();

        var response = await _client.PatchAsJsonAsync(
            $"/api/v1/organizations/{orgId}",
            new { audit_log_retention_days = 0 });

        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
        var body = await response.Content.ReadAsStringAsync();
        body.Should().Contain("retention_days_invalid");
    }
}
