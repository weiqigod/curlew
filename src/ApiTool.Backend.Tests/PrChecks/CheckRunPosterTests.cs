// Refs docs/SPECIFICATION.md:8429-8550 (outbound GitHub Checks API POST).
using System.Net;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.PrChecks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Unit tests for <see cref="CheckRunPoster"/> covering the outbound GitHub Checks API POST path.
/// Named so the filter FullyQualifiedName~CheckRunPoster matches.
/// </summary>
public sealed class CheckRunPosterTests
{
    // ── Test infrastructure helpers ────────────────────────────────────────

    private static async Task<(AppDbContext db, Guid orgId, long installationId)>
        SeedAsync(AppDbContext db, string repoSelection = "selected", string repoSetJson = """[{"id":1,"owner":"acme","name":"api"}]""")
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"user-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "TestOrg", Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        const long installId = 42L;
        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = installId, AppId = 12345L, OrgId = orgId,
            AccountLogin = "acme", AccountType = "Organization",
            RepoSelection = repoSelection, RepoSetJson = repoSetJson,
            InstalledAt = DateTime.UtcNow, LastReconciledAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, orgId, installId);
    }

    private static PrCheck MakePrCheck(Guid orgId, string repo = "acme/api", string state = "success") =>
        new()
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = repo, Pr = 1,
            State = state, HeadSha = new string('a', 40),
            CreatedAt = DateTime.UtcNow, ExternalId = Guid.NewGuid(),
        };

    private static readonly DateTimeOffset BaseTime = new(2026, 5, 7, 12, 0, 0, TimeSpan.Zero);

    private static readonly FakeGitHubAppKeyProvider DefaultKeyProvider = new("fake.jwt", 12345L);

    private static (CheckRunPoster poster, FakeHttpMessageHandler handler) BuildPoster(
        AppDbContext db,
        FakeClock? clock = null,
        HttpStatusCode githubStatus = HttpStatusCode.Created,
        string githubResponse = """{"id":999,"url":"https://api.github.com/repos/acme/api/check-runs/999"}""")
    {
        clock ??= new FakeClock(BaseTime);
        var handler = new FakeHttpMessageHandler(githubStatus, githubResponse, "application/json");
        var factory = new FakeHttpClientFactory(handler);
        var tokenCache = new InstallationTokenCache(
            new FakeHttpClientFactory(new FakeHttpMessageHandler(HttpStatusCode.Created,
                """{"token":"ghs_faketoken12345678901234567890123456","expires_at":"2099-01-01T00:00:00Z"}""",
                "application/json")),
            DefaultKeyProvider, clock, NullLogger<InstallationTokenCache>.Instance);
        var rateLimit = new RateLimitTracker(clock);
        var poster = new CheckRunPoster(db, tokenCache, rateLimit, factory, DefaultKeyProvider, clock,
            NullLogger<CheckRunPoster>.Instance);
        return (poster, handler);
    }

    private static CheckRunPoster BuildPosterWithQueuedHandler(
        AppDbContext db,
        QueuedFakeHttpMessageHandler queuedHandler,
        FakeClock clock,
        RateLimitTracker? rateLimit = null)
    {
        var factory = new FakeHttpClientFactory(queuedHandler);
        var tokenCache = new InstallationTokenCache(
            new FakeHttpClientFactory(new FakeHttpMessageHandler(HttpStatusCode.Created,
                """{"token":"ghs_faketoken12345678901234567890123456","expires_at":"2099-01-01T00:00:00Z"}""",
                "application/json")),
            DefaultKeyProvider, clock, NullLogger<InstallationTokenCache>.Instance);
        rateLimit ??= new RateLimitTracker(clock);
        return new CheckRunPoster(db, tokenCache, rateLimit, factory, DefaultKeyProvider, clock,
            NullLogger<CheckRunPoster>.Instance);
    }

    // ── Happy path ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Posts_2xx_PersistsCheckRunIdAndMarksPosted()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Posted);
        result.CheckRunId.Should().Be(999L);

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.CheckRunId.Should().Be(999L);
        updated.PostedAt.Should().NotBeNull();
        updated.Status.Should().Be("posted");
    }

    [Fact]
    public async Task TransientFiveHundred_QueuesAndRecordsLastError()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock, HttpStatusCode.InternalServerError,
            """{"message":"Internal Server Error"}""");

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Status.Should().BeOneOf("queued", "failed");
        updated.LastError.Should().NotBeNullOrEmpty();
        updated.AttemptCount.Should().Be(1);
    }

    [Fact]
    public async Task RepoNotInRepoSet_ReturnsRepoNotCovered()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, repo: "acme/other-repo"); // not in repo_set
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, handler) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.RepoNotCovered);
        handler.RequestCount.Should().Be(0); // no HTTP call made
    }

    [Fact]
    public async Task NoInstallation_ReturnsNoInstallation()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        // Org with no installation
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"u-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "NoInstOrg", Slug = $"noinst-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.NoInstallation);
    }

    [Fact]
    public async Task InstallationSuspended_ReturnsSuspended()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        // Mark installation as suspended
        var inst = await db.GithubInstallations.FirstAsync();
        inst.SuspendedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.InstallationSuspended);
    }

    [Fact]
    public async Task InstallationDeleted_ReturnsDeleted()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        // Mark installation as deleted
        var inst = await db.GithubInstallations.FirstAsync();
        inst.DeletedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.InstallationDeleted);
    }

    [Fact]
    public async Task RepoSelectionAll_BypassesRepoSetCheck()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        // Installation has repo_selection = "all" so any repo is covered
        var (_, orgId, _) = await SeedAsync(db, repoSelection: "all", repoSetJson: "[]");
        var prCheck = MakePrCheck(orgId, repo: "acme/any-random-repo");
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Posted);
    }

    [Fact]
    public async Task ConclusionMapping_AllSixStates_RoundTrip()
    {
        // Verify that the 6 states map correctly when used in PostAsync
        string[] states = ["success", "failure", "cancelled", "timed_out", "neutral", "skipped"];
        foreach (var state in states)
        {
            using var scope = TestDb.CreateOpen();
            var db = scope.Db;
            await db.Database.EnsureCreatedAsync();

            var (_, orgId, _) = await SeedAsync(db);
            var prCheck = MakePrCheck(orgId, state: state);
            db.PrChecks.Add(prCheck);
            await db.SaveChangesAsync();

            var clock = new FakeClock(BaseTime);
            var (poster, handler) = BuildPoster(db, clock);
            var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

            result.Status.Should().Be(PrCheckPostStatus.Posted, $"state '{state}' should post successfully");

            // Verify the conclusion was sent in the request body
            var requestBody = handler.LastRequestBody;
            requestBody.Should().Contain($"\"conclusion\":\"{state}\"",
                $"conclusion for state '{state}' should be '{state}'");
        }
    }

    [Fact]
    public async Task PayloadOutputSummary_Over60k_TruncatedWithMarker()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        prCheck.OutputSummary = new string('S', 61_000); // over 60k
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, handler) = BuildPoster(db, clock);

        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // The request body should have truncated summary
        handler.LastRequestBody.Should().Contain("(truncated)");
        // Verify the db row was truncated too
        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.OutputSummary!.Length.Should().BeLessOrEqualTo(MarkdownSafety.MaxField);
    }

    [Fact]
    public async Task PayloadOutputText_Over60k_TruncatedWithMarker()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        prCheck.OutputText = new string('T', 61_000);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, handler) = BuildPoster(db, clock);

        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        handler.LastRequestBody.Should().Contain("(truncated)");
    }

    // ── Token handling rules (3 negative tests per DoD) ────────────────────

    [Fact]
    public async Task InstallationToken_NeverPersisted_DbScanCleanAfterPost()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // Scan ALL nullable string columns on the row for ghs_ token pattern (finding #7)
        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.LastError.Should().NotContain("ghs_");
        row.OutputSummary.Should().BeNull();
        row.OutputText.Should().BeNull();
        row.AnnotationsJson.Should().BeNull();
        row.Conclusion.Should().NotContain("ghs_");
        row.DetailsUrl.Should().BeNull();
        row.HeadSha.Should().NotContain("ghs_");
    }

    [Fact]
    public async Task InstallationToken_NeverEchoedInResponse()
    {
        // The response from CheckRunPoster does not include the ghs_ token
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // The result record must not contain any ghs_ tokens
        result.Error.Should().NotContain("ghs_");
    }

    // ── Finding #1: Idempotency GET on crash-recovery path ────────────────────

    [Fact]
    public async Task RetryAfterCrash_FindsExistingRunViaGet_DoesNotDoublePost()
    {
        // Row has PostingStartedAt set but PostedAt null → crash recovery path.
        // GET /repos/{o}/{r}/check-runs?head_sha=...&app_id=...&filter=latest returns
        // a check run whose external_id matches → should return AlreadyPosted without POSTing.
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        prCheck.PostingStartedAt = BaseTime.AddMinutes(-1).UtcDateTime; // crash-recovery: posting started but not finished
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);

        // First call = GET idempotency check (returns a run with matching external_id)
        // No second call should happen (no POST)
        var getResponse = $$$"""{"total_count":1,"check_runs":[{"id":777,"external_id":"{{{prCheck.ExternalId}}}","status":"completed"}]}""";
        var queuedHandler = new QueuedFakeHttpMessageHandler()
            .Enqueue(HttpStatusCode.OK, getResponse);

        var poster = BuildPosterWithQueuedHandler(db, queuedHandler, clock);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.AlreadyPosted);
        result.CheckRunId.Should().Be(777L);
        queuedHandler.RequestCount.Should().Be(1, "only the GET should have been called, not a POST");

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Status.Should().Be("posted");
        updated.CheckRunId.Should().Be(777L);
    }

    // ── Finding #4b: Rate-limit queuing (X-RateLimit-Remaining < 100) ─────────

    [Fact]
    public async Task RateLimitRemainingBelow100_QueuesNewPosts()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, installationId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var rateLimit = new RateLimitTracker(clock);

        // Pre-block the installation: remaining=50 (below 100) with a reset time in the future
        rateLimit.Update(installationId, remaining: 50, retryAfter: null, resetAt: clock.GetUtcNow().AddMinutes(5));

        var queuedHandler = new QueuedFakeHttpMessageHandler(); // no responses queued — should not be called
        var poster = BuildPosterWithQueuedHandler(db, queuedHandler, clock, rateLimit);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);
        result.Code.Should().Be(PrCheckErrorCode.GithubRateLimited);
        queuedHandler.RequestCount.Should().Be(0, "no HTTP call should be made when rate-limited");

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Status.Should().Be("queued");
    }

    // ── Finding #4c: Secondary rate-limit 403 + Retry-After blocks installation ─

    [Fact]
    public async Task SecondaryRateLimit403WithRetryAfter_BlocksInstallation()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, installationId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var rateLimit = new RateLimitTracker(clock);

        // GitHub returns 403 + Retry-After header (secondary rate limit)
        var handler403 = new FakeHttpMessageHandler(HttpStatusCode.Forbidden,
            """{"message":"You have exceeded a secondary rate limit"}""");
        handler403.ResponseHeaders["Retry-After"] = "30";

        var factory = new FakeHttpClientFactory(handler403);
        var tokenCache = new InstallationTokenCache(
            new FakeHttpClientFactory(new FakeHttpMessageHandler(HttpStatusCode.Created,
                """{"token":"ghs_faketoken12345678901234567890123456","expires_at":"2099-01-01T00:00:00Z"}""",
                "application/json")),
            DefaultKeyProvider, clock, NullLogger<InstallationTokenCache>.Instance);
        var poster = new CheckRunPoster(db, tokenCache, rateLimit, factory, DefaultKeyProvider, clock,
            NullLogger<CheckRunPoster>.Instance);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);
        result.Code.Should().Be(PrCheckErrorCode.GithubRateLimited);

        // Verify the installation is now blocked
        rateLimit.IsBlocked(installationId).Should().BeTrue(
            "installation should be blocked after 403+Retry-After");

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Status.Should().Be("queued");
    }

    // ── Finding #1 (review iter 2): GitHub 422 must map to PermanentFailure ─────

    [Fact]
    public async Task GitHub422_UnprocessableEntity_ReturnsPermanentFailure()
    {
        // A 422 from GitHub means our payload is malformed — it is NOT a repo-access
        // issue and must not be mapped to RepoNotCovered.
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock,
            HttpStatusCode.UnprocessableEntity,
            """{"message":"Validation Failed"}""");

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.PermanentFailure,
            "422 is a payload schema error, not a repo-access error");

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Status.Should().Be("failed");
    }

    [Fact]
    public async Task GitHub404_NotFound_ReturnsRepoNotCovered()
    {
        // A 404 from GitHub on the check-runs POST means the repo is not found
        // in the installation — should map to RepoNotCovered.
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var (poster, _) = BuildPoster(db, clock,
            HttpStatusCode.NotFound,
            """{"message":"Not Found"}""");

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.RepoNotCovered);
    }

    // ── Finding #2 (review iter 2): Idempotency path must set row.Conclusion ──

    [Fact]
    public async Task RetryAfterCrash_IdempotencyFound_SetsConclusion()
    {
        // When the idempotency GET finds an existing run, the db row should have
        // Conclusion populated — not null — so the record is consistent.
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, state: "success");
        prCheck.PostingStartedAt = BaseTime.AddMinutes(-1).UtcDateTime;
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var getResponse = $$$"""{"total_count":1,"check_runs":[{"id":888,"external_id":"{{{prCheck.ExternalId}}}","status":"completed"}]}""";
        var queuedHandler = new QueuedFakeHttpMessageHandler()
            .Enqueue(HttpStatusCode.OK, getResponse);
        var poster = BuildPosterWithQueuedHandler(db, queuedHandler, clock);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.AlreadyPosted);

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.Conclusion.Should().Be("success",
            "conclusion must be set on the idempotency-found path, not left null");
    }

    [Fact]
    public async Task InstallationToken_NeverWrittenToLogger()
    {
        // Use a recording logger to verify no ghs_ tokens are written
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var (_, orgId, _) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var clock = new FakeClock(BaseTime);
        var recordingLogger = new RecordingLogger<CheckRunPoster>();
        var handler = new FakeHttpMessageHandler(HttpStatusCode.Created,
            """{"id":999,"url":"https://api.github.com/repos/acme/api/check-runs/999"}""",
            "application/json");
        var tokenHandler = new FakeHttpMessageHandler(HttpStatusCode.Created,
            """{"token":"ghs_logtest12345678901234567890123456789","expires_at":"2099-01-01T00:00:00Z"}""",
            "application/json");
        var factory = new FakeHttpClientFactory(handler);
        var tokenCache = new InstallationTokenCache(
            new FakeHttpClientFactory(tokenHandler), DefaultKeyProvider, clock,
            NullLogger<InstallationTokenCache>.Instance);
        var rateLimit = new RateLimitTracker(clock);
        var poster = new CheckRunPoster(db, tokenCache, rateLimit, factory, DefaultKeyProvider, clock, recordingLogger);

        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // No log message should contain the ghs_ token pattern
        recordingLogger.Messages.Should().NotContain(msg => msg.Contains("ghs_"));
    }
}
