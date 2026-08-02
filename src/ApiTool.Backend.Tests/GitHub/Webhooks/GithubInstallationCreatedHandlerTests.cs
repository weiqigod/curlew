// Refs docs/SPECIFICATION.md:8416-8417 (webhook-first install path).
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubInstallationCreatedHandler: webhook-first install path.
/// </summary>
public sealed class GithubInstallationCreatedHandlerTests
{
    [Fact]
    public async Task installation_created_with_no_pending_claim_inserts_row_with_null_org_id()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var clock = new FakeClock(DateTimeOffset.UtcNow);
        var svc = new GithubInstallationsService(
            db, clock, new FakeInstallationTokenCache(),
            NullLogger<GithubInstallationsService>.Instance);
        var handler = new GithubInstallationCreatedHandler(svc);

        var payload = new GithubInstallationCreatedPayload(
            InstallationId: 77001L,
            AppId: 12345L,
            AccountLogin: "acme-corp",
            AccountType: "Organization",
            RepoSelection: "selected",
            Repositories: [new RepoRef(1, "acme", "api"), new RepoRef(2, "acme", "sdk")]);

        await handler.HandleAsync(payload, CancellationToken.None);

        var row = await db.GithubInstallations.FindAsync(77001L);
        row.Should().NotBeNull();
        row!.OrgId.Should().BeNull();
        row.ClaimedAt.Should().BeNull();
        row.AccountLogin.Should().Be("acme-corp");
    }
}
