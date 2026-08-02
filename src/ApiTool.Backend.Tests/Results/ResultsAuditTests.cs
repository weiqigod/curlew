using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Results;

/// <summary>
/// Integration tests verifying that <c>IngestResult</c> emits exactly the right
/// audit row — one on success, none on permission-denied, and none on invalid-schema.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class ResultsAuditTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _ownerClient;
    private readonly Guid _ownerId;
    private string? _orgId;

    public ResultsAuditTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var email = $"audit-owner-{_ownerId:N}@example.com";
        var token = TestTokens.Create(_ownerId, email);

        _ownerClient = factory.CreateClient();
        _ownerClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();

        // Create an org for this test instance
        var slug = $"au-{_ownerId:N}"[..20];
        var body = new { name = "AuditTestOrg", slug };
        var response = await _ownerClient.PostAsJsonAsync("/api/v1/organizations", body);
        response.EnsureSuccessStatusCode();
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    private object ValidPayload() => new
    {
        collection_name = "audit-test-collection",
        run_at = "2026-04-19T12:00:00Z",
        duration_ms = 1234,
        pass_count = 2,
        fail_count = 0,
        skipped_count = 0,
        triggered_by = "cli",
        git_sha = "abc123",
        items = new[]
        {
            new { name = "test-1", status = "passed", duration_ms = 100, message = (string?)null },
            new { name = "test-2", status = "passed", duration_ms = 100, message = (string?)null },
        },
    };

    // ── Test 1: success_emits_row ─────────────────────────────────────────────

    [Fact]
    public async Task Ingest_success_emits_results_upload_audit_row()
    {
        // act
        var response = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload());

        // assert HTTP
        response.StatusCode.Should().Be(HttpStatusCode.Accepted);
        var json = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        var resultId = doc.RootElement.GetProperty("result_id").GetString()!;

        // assert audit row
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();

        // Parse the org GUID from "org_<hex>" wire format
        var orgGuid = ApiTool.Backend.Organizations.OrgId.TryParse(_orgId!, out var g) ? g : Guid.Empty;
        orgGuid.Should().NotBe(Guid.Empty, because: "org id must parse cleanly");

        var entry = await db.OrganizationAuditLog
            .OrderByDescending(e => e.CreatedAt)
            .FirstOrDefaultAsync(e => e.OrgId == orgGuid && e.EventType == "results.upload");

        entry.Should().NotBeNull(because: "a results.upload audit row must be written on successful ingest");
        entry!.EventType.Should().Be("results.upload");
        entry.TargetType.Should().Be("result");
        entry.Success.Should().BeTrue();
        // TargetId is the raw GUID extracted from the res_<hex> wire format
        var wireHex = resultId["res_".Length..];
        var targetGuid = Guid.ParseExact(wireHex, "N");
        entry.TargetId.Should().Be(targetGuid);
        // Regression for M5-021: the audit row must carry the actor email so the
        // audit-log UI can render `qa@acme.example` rather than `—`. The owner email
        // is set by CurrentUserAccessor when it upserts the user row from the JWT,
        // so we assert it is non-empty rather than pinning a value (the email is
        // synthesized in the test fixture).
        entry.ActorEmail.Should().NotBeNullOrEmpty();
        entry.ActorEmail.Should().Contain("@");
    }

    // ── Test 2: permission_denied_no_row ──────────────────────────────────────

    [Fact]
    public async Task Ingest_permission_denied_emits_no_audit_row()
    {
        // arrange: an outsider who is not a member of the org
        var (outsiderToken, _) = TestTokens.CreateNew($"outsider-{Guid.NewGuid():N}@example.com");
        var outsiderClient = _factory.CreateClient();
        outsiderClient.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", outsiderToken);

        // Count rows before the call
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var orgGuid = ApiTool.Backend.Organizations.OrgId.TryParse(_orgId!, out var g) ? g : Guid.Empty;
        var countBefore = await db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgGuid && e.EventType == "results.upload");

        // act
        var response = await outsiderClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", ValidPayload());

        // assert HTTP
        response.StatusCode.Should().Be(HttpStatusCode.Forbidden);

        // assert no new audit row
        var countAfter = await db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgGuid && e.EventType == "results.upload");
        countAfter.Should().Be(countBefore, because: "no audit row must be written when permission is denied");
    }

    // ── Test 3: invalid_schema_no_row ─────────────────────────────────────────

    [Fact]
    public async Task Ingest_invalid_schema_emits_no_audit_row()
    {
        // arrange: owner sends a payload missing pass_count (required)
        var badPayload = new
        {
            collection_name = "audit-test-collection",
            run_at = "2026-04-19T12:00:00Z",
            duration_ms = 100,
            // pass_count intentionally omitted → invalid schema
            fail_count = 0,
            items = Array.Empty<object>(),
        };

        // Count rows before the call
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<ApiTool.Backend.Data.AppDbContext>();
        var orgGuid = ApiTool.Backend.Organizations.OrgId.TryParse(_orgId!, out var g) ? g : Guid.Empty;
        var countBefore = await db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgGuid && e.EventType == "results.upload");

        // act
        var response = await _ownerClient.PostAsJsonAsync(
            $"/api/v1/organizations/{_orgId}/results", badPayload);

        // assert HTTP
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);

        // assert no new audit row
        var countAfter = await db.OrganizationAuditLog
            .CountAsync(e => e.OrgId == orgGuid && e.EventType == "results.upload");
        countAfter.Should().Be(countBefore, because: "no audit row must be written when schema validation fails");
    }
}
