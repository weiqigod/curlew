// Refs docs/SPECIFICATION.md:8425 (daily reconciliation host).
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.GitHub.Installations;

/// <summary>
/// Tests for GithubInstallationReconcilerHost using a fake IGitHubInstallationsApi.
/// Uses a SQLite in-memory DB so FK constraints are enforced.
/// </summary>
public sealed class GithubInstallationReconcilerHostTests
{
    private const long AppId = 12345L;

    private static async Task<(AppDbContext db, Guid orgId, long installId)> SeedAsync(
        TestDbScope scope, bool suspended = false, bool deleted = false)
    {
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        const long installId = 99001L;

        db.Users.Add(new User { Id = userId, Email = $"rec-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Rec", Slug = $"rec-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = installId,
            AppId = AppId,
            OrgId = orgId,
            AccountLogin = "acme-corp",
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = DateTime.UtcNow,
            ClaimedAt = DateTime.UtcNow,
            SuspendedAt = suspended ? DateTime.UtcNow : null,
            DeletedAt = deleted ? DateTime.UtcNow : null,
            LastReconciledAt = DateTime.UtcNow.AddDays(-2),
        });
        await db.SaveChangesAsync();
        return (db, orgId, installId);
    }

    private static GithubInstallationReconcilerHost CreateHost(
        IServiceScopeFactory scopeFactory,
        IGitHubInstallationsApi api,
        TimeProvider? clock = null)
    {
        clock ??= new FakeClock(DateTimeOffset.UtcNow);
        var options = Options.Create(new GithubInstallationReconcilerOptions());

        return new GithubInstallationReconcilerHost(
            scopeFactory, api, options, clock,
            NullLogger<GithubInstallationReconcilerHost>.Instance,
            tickInterval: TimeSpan.FromMilliseconds(50));
    }

    [Fact]
    public async Task Reconciler_corrects_repo_set_drift_for_active_install()
    {
        // Use a shared connection so multiple AppDbContext instances see the same data
        var conn = new Microsoft.Data.Sqlite.SqliteConnection("DataSource=:memory:");
        conn.Open();
        try
        {
            long installId;
            using (var seedScope = TestDb.CreateOpenFromConnection(conn))
            {
                var (db, _, id) = await SeedAsync(seedScope);
                installId = id;
            }

            var githubRepos = new[] { new RepoRef(1001, "acme", "api"), new RepoRef(1002, "acme", "sdk") };
            var fakeApi = new FakeGitHubInstallationsApi(installId, githubRepos);

            var services = new ServiceCollection();
            services.AddScoped<AppDbContext>(_ =>
            {
                var s = TestDb.CreateOpenFromConnection(conn);
                return s.Db; // each scope gets its own context on the shared connection
            });
            services.AddSingleton(TimeProvider.System);
            services.AddSingleton<IInstallationTokenCache, FakeInstallationTokenCache>();
            services.AddScoped<GithubInstallationsService>();
            services.AddLogging();
            using var sp = services.BuildServiceProvider();

            var host = CreateHost(sp.GetRequiredService<IServiceScopeFactory>(), fakeApi);
            using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

            await host.StartAsync(cts.Token);
            await Task.Delay(200, CancellationToken.None); // give it one tick
            cts.Cancel();
            try { await host.StopAsync(CancellationToken.None); } catch (OperationCanceledException) { }

            // Read results via a fresh context on the same connection
            using var checkScope = TestDb.CreateOpenFromConnection(conn);
            var row = await checkScope.Db.GithubInstallations.FindAsync(installId);
            var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(
                row!.RepoSetJson,
                new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
            list.Should().HaveCount(2);
            list!.Select(r => r.Id).Should().Contain(1001).And.Contain(1002);
        }
        finally
        {
            await conn.DisposeAsync();
        }
    }

    [Fact]
    public async Task Reconciler_skips_suspended_installations()
    {
        var conn = new Microsoft.Data.Sqlite.SqliteConnection("DataSource=:memory:");
        conn.Open();
        try
        {
            long installId;
            using (var seedScope = TestDb.CreateOpenFromConnection(conn))
                (_, _, installId) = await SeedAsync(seedScope, suspended: true);

            var githubRepos = new[] { new RepoRef(2001, "acme", "api") };
            var fakeApi = new FakeGitHubInstallationsApi(installId, githubRepos);

            var services = new ServiceCollection();
            services.AddScoped<AppDbContext>(_ => TestDb.CreateOpenFromConnection(conn).Db);
            services.AddSingleton(TimeProvider.System);
            services.AddSingleton<IInstallationTokenCache, FakeInstallationTokenCache>();
            services.AddScoped<GithubInstallationsService>();
            services.AddLogging();
            using var sp = services.BuildServiceProvider();

            var host = CreateHost(sp.GetRequiredService<IServiceScopeFactory>(), fakeApi);
            using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

            await host.StartAsync(cts.Token);
            await Task.Delay(200, CancellationToken.None);
            cts.Cancel();
            try { await host.StopAsync(CancellationToken.None); } catch (OperationCanceledException) { }

            // API should NOT have been called (suspended installation)
            fakeApi.CallCount.Should().Be(0);
        }
        finally { await conn.DisposeAsync(); }
    }

    [Fact]
    public async Task Reconciler_skips_soft_deleted_installations()
    {
        var conn = new Microsoft.Data.Sqlite.SqliteConnection("DataSource=:memory:");
        conn.Open();
        try
        {
            long installId;
            using (var seedScope = TestDb.CreateOpenFromConnection(conn))
                (_, _, installId) = await SeedAsync(seedScope, deleted: true);

            var githubRepos = new[] { new RepoRef(3001, "acme", "api") };
            var fakeApi = new FakeGitHubInstallationsApi(installId, githubRepos);

            var services = new ServiceCollection();
            services.AddScoped<AppDbContext>(_ => TestDb.CreateOpenFromConnection(conn).Db);
            services.AddSingleton(TimeProvider.System);
            services.AddSingleton<IInstallationTokenCache, FakeInstallationTokenCache>();
            services.AddScoped<GithubInstallationsService>();
            services.AddLogging();
            using var sp = services.BuildServiceProvider();

            var host = CreateHost(sp.GetRequiredService<IServiceScopeFactory>(), fakeApi);
            using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

            await host.StartAsync(cts.Token);
            await Task.Delay(200, CancellationToken.None);
            cts.Cancel();
            try { await host.StopAsync(CancellationToken.None); } catch (OperationCanceledException) { }

            fakeApi.CallCount.Should().Be(0);
        }
        finally { await conn.DisposeAsync(); }
    }

    [Fact]
    public async Task Reconciler_updates_last_reconciled_at()
    {
        var conn = new Microsoft.Data.Sqlite.SqliteConnection("DataSource=:memory:");
        conn.Open();
        try
        {
            long installId;
            DateTime before;
            using (var seedScope = TestDb.CreateOpenFromConnection(conn))
            {
                var (db, _, id) = await SeedAsync(seedScope);
                installId = id;
                before = db.GithubInstallations.Find(installId)!.LastReconciledAt;
            }

            var githubRepos = new[] { new RepoRef(4001, "acme", "api") };
            var fakeApi = new FakeGitHubInstallationsApi(installId, githubRepos);

            var services = new ServiceCollection();
            services.AddScoped<AppDbContext>(_ => TestDb.CreateOpenFromConnection(conn).Db);
            services.AddSingleton(TimeProvider.System);
            services.AddSingleton<IInstallationTokenCache, FakeInstallationTokenCache>();
            services.AddScoped<GithubInstallationsService>();
            services.AddLogging();
            using var sp = services.BuildServiceProvider();

            var host = CreateHost(sp.GetRequiredService<IServiceScopeFactory>(), fakeApi);
            using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

            await Task.Delay(10, CancellationToken.None); // ensure 'before' is before reconciliation
            await host.StartAsync(cts.Token);
            await Task.Delay(300, CancellationToken.None);
            cts.Cancel();
            try { await host.StopAsync(CancellationToken.None); } catch (OperationCanceledException) { }

            using var checkScope = TestDb.CreateOpenFromConnection(conn);
            var row = await checkScope.Db.GithubInstallations.FindAsync(installId);
            row!.LastReconciledAt.Should().BeAfter(before);
        }
        finally
        {
            await conn.DisposeAsync();
        }
    }

    [Fact]
    public async Task Reconciler_removes_repos_absent_from_github_response()
    {
        // Arrange: seed an installation with repos [A, B] in repo_set
        var conn = new Microsoft.Data.Sqlite.SqliteConnection("DataSource=:memory:");
        conn.Open();
        try
        {
            long installId;
            using (var seedScope = TestDb.CreateOpenFromConnection(conn))
            {
                var (db, _, id) = await SeedAsync(seedScope);
                installId = id;

                // Pre-populate repo_set with two repos
                var row = db.GithubInstallations.Find(installId)!;
                var twoRepos = new[]
                {
                    new RepoRef(9001, "acme", "api"),
                    new RepoRef(9002, "acme", "sdk"),
                };
                row.RepoSetJson = System.Text.Json.JsonSerializer.Serialize(
                    twoRepos,
                    new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
                await db.SaveChangesAsync();
            }

            // GitHub now only returns [A]; B should be removed during reconciliation
            var githubRepos = new[] { new RepoRef(9001, "acme", "api") };
            var fakeApi = new FakeGitHubInstallationsApi(installId, githubRepos);

            var services = new ServiceCollection();
            services.AddScoped<AppDbContext>(_ => TestDb.CreateOpenFromConnection(conn).Db);
            services.AddSingleton(TimeProvider.System);
            services.AddSingleton<IInstallationTokenCache, FakeInstallationTokenCache>();
            services.AddScoped<GithubInstallationsService>();
            services.AddLogging();
            using var sp = services.BuildServiceProvider();

            var host = CreateHost(sp.GetRequiredService<IServiceScopeFactory>(), fakeApi);
            using var cts = new CancellationTokenSource(TimeSpan.FromSeconds(5));

            await host.StartAsync(cts.Token);
            await Task.Delay(200, CancellationToken.None);
            cts.Cancel();
            try { await host.StopAsync(CancellationToken.None); } catch (OperationCanceledException) { }

            // Assert: repo_set should only contain [A]; B must be gone
            using var checkScope = TestDb.CreateOpenFromConnection(conn);
            var result = await checkScope.Db.GithubInstallations.FindAsync(installId);
            var list = System.Text.Json.JsonSerializer.Deserialize<List<RepoRef>>(
                result!.RepoSetJson,
                new System.Text.Json.JsonSerializerOptions(System.Text.Json.JsonSerializerDefaults.Web));
            list.Should().HaveCount(1, "reconciler must remove repos absent from GitHub response");
            list![0].Id.Should().Be(9001);
        }
        finally
        {
            await conn.DisposeAsync();
        }
    }

    // ── Fake ───────────────────────────────────────────────────────────────────

    private sealed class FakeGitHubInstallationsApi : IGitHubInstallationsApi
    {
        private readonly long _installationId;
        private readonly IReadOnlyList<RepoRef> _repos;
        public int CallCount { get; private set; }

        public FakeGitHubInstallationsApi(long installationId, IReadOnlyList<RepoRef> repos)
        {
            _installationId = installationId;
            _repos = repos;
        }

        public Task<IReadOnlyList<RepoRef>> ListInstallationRepositoriesAsync(
            long installationId, CancellationToken ct)
        {
            if (installationId == _installationId)
                CallCount++;
            return Task.FromResult(_repos);
        }
    }
}
