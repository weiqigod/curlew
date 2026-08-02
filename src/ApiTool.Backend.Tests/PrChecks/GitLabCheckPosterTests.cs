// Refs docs/SPECIFICATION.md:9216-9246 (outbound GitLab Commit Status API).
using System.Net;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.PrChecks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.PrChecks;

/// <summary>
/// Unit tests for <see cref="GitLabCheckPoster"/> covering the outbound GitLab Commit Status
/// API POST path. Named so the filter FullyQualifiedName~GitLabCheckPoster matches.
/// </summary>
public sealed class GitLabCheckPosterTests
{
    // ── Test infrastructure helpers ────────────────────────────────────────

    private const string FakePat = "glpat-fake1234567890";
    private static readonly DateTimeOffset BaseTime = new(2026, 5, 11, 12, 0, 0, TimeSpan.Zero);
    private static readonly FakeGitLabKeyProvider DefaultKeyProvider = new();

    private static async Task<(AppDbContext db, Guid orgId, Guid installationId)>
        SeedAsync(AppDbContext db, string gitLabBaseUrl = "https://gitlab.com")
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"gl-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "GLTestOrg", Slug = $"gl-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active, CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        var installId = Guid.NewGuid();
        var patBytes = System.Text.Encoding.UTF8.GetBytes(FakePat);
        var enc = await DefaultKeyProvider.EncryptAsync(patBytes);
        db.GitLabInstallations.Add(new GitLabInstallation
        {
            Id = installId, OrgId = orgId, ProjectId = 100L, ProjectPath = "group/proj",
            GitLabBaseUrl = gitLabBaseUrl,
            AccessTokenCiphertext = enc.Ciphertext,
            AccessTokenKid = enc.Kid,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (db, orgId, installId);
    }

    private static PrCheck MakePrCheck(Guid orgId, Guid installationId, string state = "success") =>
        new()
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "group/proj", Pr = 1,
            Provider = "gitlab", State = state, HeadSha = new string('a', 40),
            CreatedAt = DateTime.UtcNow, ExternalId = Guid.NewGuid(),
            GitLabInstallationId = installationId,
        };

    private static (GitLabCheckPoster poster, FakeHttpMessageHandler handler) BuildPoster(
        AppDbContext db,
        FakeClock? clock = null,
        HttpStatusCode gitlabStatus = HttpStatusCode.Created,
        string gitlabResponse = """{"id":9001,"sha":"aaa","ref":"main","status":"success","name":"ApiTool"}""",
        bool allowHttp = false)
    {
        clock ??= new FakeClock(BaseTime);
        var handler = new FakeHttpMessageHandler(gitlabStatus, gitlabResponse);
        var factory = new FakeHttpClientFactory(handler, baseAddress: null); // GitLab uses full URLs
        var rateLimit = new GitLabRateLimitTracker(clock);
        var options = Options.Create(new GitLabOptions { Poster = new GitLabOptions.PosterConfig { AllowHttp = allowHttp } });
        var audit = new NullAuditWriter();
        var poster = new GitLabCheckPoster(db, DefaultKeyProvider, rateLimit, factory, options, audit, clock,
            NullLogger<GitLabCheckPoster>.Instance);
        return (poster, handler);
    }

    // ── Happy path ──────────────────────────────────────────────────────────

    [Fact]
    public async Task Posts_2xx_PersistsGitLabStatusIdAndMarksPosted()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Posted);

        db.ChangeTracker.Clear();
        var updated = await db.PrChecks.FindAsync(prCheck.Id);
        updated!.GitLabStatusId.Should().Be(9001L);
        updated.PostedAt.Should().NotBeNull();
        updated.Status.Should().Be("posted");
    }

    [Fact]
    public async Task PostsSuccess_BodyContains_State_Name_Description_Context()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId, "success");
        prCheck.DetailsUrl = "https://app.apitool.dev/runs/r1";
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        using var doc = JsonDocument.Parse(handler.LastRequestBody);
        doc.RootElement.GetProperty("state").GetString().Should().Be("success");
        doc.RootElement.GetProperty("name").GetString().Should().Be("ApiTool");
        doc.RootElement.GetProperty("context").GetString().Should().Be("ci/apitool");
        doc.RootElement.GetProperty("target_url").GetString().Should().Be("https://app.apitool.dev/runs/r1");
    }

    [Fact]
    public async Task PostsSuccess_PrivateTokenHeader_IsDecryptedPat()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // FakeGitLabKeyProvider is an identity provider — ciphertext bytes == plaintext bytes == FakePat.
        // FakeHttpMessageHandler.LastRequestHeaders captures all non-standard headers including Private-Token.
        handler.LastRequestHeaders.Should().ContainKey("Private-Token");
        handler.LastRequestHeaders["Private-Token"].Should().Be(FakePat);
    }

    [Theory]
    [InlineData("success",   "success",  "")]
    [InlineData("failure",   "failed",   "")]
    [InlineData("cancelled", "canceled", "")]
    [InlineData("timed_out", "failed",   "[timed out] ")]
    [InlineData("neutral",   "success",  "neutral: ")]
    [InlineData("skipped",   "success",  "skipped: ")]
    public async Task StateMapping_TableDriven(string cliState, string expectedGlState, string expectedPrefix)
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId, cliState);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);
        result.Status.Should().Be(PrCheckPostStatus.Posted, $"state '{cliState}' should post");

        using var doc = JsonDocument.Parse(handler.LastRequestBody);
        doc.RootElement.GetProperty("state").GetString().Should().Be(expectedGlState);
        if (!string.IsNullOrEmpty(expectedPrefix))
            doc.RootElement.GetProperty("description").GetString().Should().StartWith(expectedPrefix);
    }

    [Fact]
    public async Task Description_LongerThan255_TruncatedWithMarker()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId, "timed_out");
        prCheck.OutputSummary = new string('x', 300);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        using var doc = JsonDocument.Parse(handler.LastRequestBody);
        var desc = doc.RootElement.GetProperty("description").GetString()!;
        desc.Length.Should().BeLessOrEqualTo(255);
        desc.Should().EndWith("…(truncated)");
    }

    // ── 401 token revoked path ──────────────────────────────────────────────

    [Fact]
    public async Task Returns401_MarksAccessTokenRevoked_AndSetsStatus_GitLabTokenRevoked()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var spy = new SpyAuditWriter();
        var clock = new FakeClock(BaseTime);
        var handler401 = new FakeHttpMessageHandler(HttpStatusCode.Unauthorized,
            """{"message":"401 Unauthorized"}""");
        var factory = new FakeHttpClientFactory(handler401, baseAddress: null);
        var rateLimit = new GitLabRateLimitTracker(clock);
        var options = Options.Create(new GitLabOptions());
        var poster = new GitLabCheckPoster(db, DefaultKeyProvider, rateLimit, factory, options,
            spy, clock, NullLogger<GitLabCheckPoster>.Instance);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabTokenRevoked);

        db.ChangeTracker.Clear();
        var inst = await db.GitLabInstallations.FindAsync(installId);
        inst!.AccessTokenRevokedAt.Should().NotBeNull();

        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.Status.Should().Be("gitlab_token_revoked");

        // Behavior #6 (Decision F): audit log must record the revocation event
        spy.Events.Should().ContainSingle(e => e.EventType == "gitlab.pat.revoked",
            "a 401 from GitLab should append an audit event with event_type 'gitlab.pat.revoked'");
    }

    // ── 429 rate-limit path ──────────────────────────────────────────────────

    [Fact]
    public async Task Returns429_QueuesRow_AndUpdatesRateLimitTracker()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var handler = new FakeHttpMessageHandler(HttpStatusCode.TooManyRequests,
            """{"message":"429 Too Many Requests"}""");
        handler.ResponseHeaders["Retry-After"] = "30";
        var factory = new FakeHttpClientFactory(handler);
        var clock = new FakeClock(BaseTime);
        var rateLimit = new GitLabRateLimitTracker(clock);
        var options = Options.Create(new GitLabOptions());
        var poster = new GitLabCheckPoster(db, DefaultKeyProvider, rateLimit, factory, options,
            new NullAuditWriter(), clock, NullLogger<GitLabCheckPoster>.Instance);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);
        result.Code.Should().Be(PrCheckErrorCode.GitLabRateLimited);
        rateLimit.IsBlocked(installId).Should().BeTrue("installation should be blocked after 429");

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.Status.Should().Be("queued");
    }

    // ── 404 / 422 permanent failure ───────────────────────────────────────────

    [Fact]
    public async Task Returns404_MarksFailed_Permanent()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db, gitlabStatus: HttpStatusCode.NotFound,
            gitlabResponse: """{"message":"Not Found"}""");
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabPermanentFailure);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.Status.Should().Be("failed");
    }

    [Fact]
    public async Task Returns422_MarksFailed_Permanent()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db, gitlabStatus: HttpStatusCode.UnprocessableEntity,
            gitlabResponse: """{"message":"Unprocessable Entity"}""");
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabPermanentFailure);
    }

    // ── 5xx transient / retry ──────────────────────────────────────────────────

    [Fact]
    public async Task Returns5xx_QueuesForRetry()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        // AttemptCount starts at 0 — first attempt should queue, not permanently fail
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db, gitlabStatus: HttpStatusCode.ServiceUnavailable,
            gitlabResponse: """{"message":"503 Service Unavailable"}""");
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);
        result.Code.Should().Be(PrCheckErrorCode.GitLabUnreachable);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.Status.Should().Be("queued", "first 5xx attempt should queue for retry, not permanently fail");
    }

    [Fact]
    public async Task Returns5xx_MarksFailed_AfterFiveAttempts()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        // Seed a PrCheck that has already been attempted 4 times (AttemptCount=4)
        var prCheck = MakePrCheck(orgId, installId);
        prCheck.AttemptCount = 4;
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db, gitlabStatus: HttpStatusCode.ServiceUnavailable,
            gitlabResponse: """{"message":"503 Service Unavailable"}""");
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        // After the 5th attempt (AttemptCount incremented to 5 in PostAsync), should permanently fail
        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabPermanentFailure);

        db.ChangeTracker.Clear();
        var row = await db.PrChecks.FindAsync(prCheck.Id);
        row!.Status.Should().Be("failed", "5xx after 5 attempts should permanently fail");
        row.AttemptCount.Should().Be(5);
    }

    // ── HTTP insecure URL enforcement ─────────────────────────────────────────

    [Fact]
    public async Task HttpBaseUrl_WithoutAllowHttp_RejectsBeforeNetworkCall()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db, "http://gitlab.internal");
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db, allowHttp: false);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabHttpInsecure);
        handler.RequestCount.Should().Be(0, "no HTTP call should be made");
    }

    [Fact]
    public async Task HttpBaseUrl_WithAllowHttpTrue_AllowsHttp()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db, "http://gitlab.internal");
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db, allowHttp: true);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Posted);
        handler.RequestCount.Should().Be(1);
    }

    [Fact]
    public async Task HttpsBaseUrl_Always_Allowed()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db, "https://gitlab.com");
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, _) = BuildPoster(db, allowHttp: false);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Posted);
    }

    // ── target_url HTTPS validation ────────────────────────────────────────────

    [Fact]
    public async Task DetailsUrl_NotHttps_DroppedFromPayload()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        prCheck.DetailsUrl = "http://insecure.example.com/run";
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        using var doc = JsonDocument.Parse(handler.LastRequestBody);
        doc.RootElement.TryGetProperty("target_url", out var p).Should().BeFalse(
            "http details_url must be dropped");
    }

    [Fact]
    public async Task DetailsUrl_Https_IncludedAsTargetUrl()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        prCheck.DetailsUrl = "https://app.apitool.dev/runs/123";
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        await poster.PostAsync(prCheck.Id, CancellationToken.None);

        using var doc = JsonDocument.Parse(handler.LastRequestBody);
        doc.RootElement.GetProperty("target_url").GetString()
            .Should().Be("https://app.apitool.dev/runs/123");
    }

    // ── Pre-existing revoked state ─────────────────────────────────────────────

    [Fact]
    public async Task AccessTokenRevokedAt_AlreadySet_ShortCircuits_WithoutHttpCall()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        // Mark the installation as already revoked
        var inst = await db.GitLabInstallations.FindAsync(installId);
        inst!.AccessTokenRevokedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabTokenRevoked);
        handler.RequestCount.Should().Be(0, "no HTTP call should be made for already-revoked token");
    }

    [Fact]
    public async Task InstallationSoftDeleted_ReturnsFailed_GitLabNoInstallation()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        // Soft-delete the installation
        var inst = await db.GitLabInstallations.FindAsync(installId);
        inst!.DeletedAt = DateTime.UtcNow;
        await db.SaveChangesAsync();

        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabNoInstallation);
        handler.RequestCount.Should().Be(0, "no HTTP call should be made for a soft-deleted installation");
    }

    [Fact]
    public async Task NoGitLabInstallation_ReturnsFailed_GitLabNoInstallation()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, _) = await SeedAsync(db);
        // Create a pr_check with no installation id
        var prCheck = new PrCheck
        {
            Id = Guid.NewGuid(), OrgId = orgId, Repo = "g/p", Pr = 1,
            Provider = "gitlab", State = "success", HeadSha = new string('c', 40),
            CreatedAt = DateTime.UtcNow, ExternalId = Guid.NewGuid(),
            GitLabInstallationId = null,
        };
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var (poster, handler) = BuildPoster(db);
        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Failed);
        result.Code.Should().Be(PrCheckErrorCode.GitLabNoInstallation);
        handler.RequestCount.Should().Be(0);
    }

    // ── Pre-blocked installation ────────────────────────────────────────────────

    [Fact]
    public async Task RateLimitedInstallation_Queues_WithoutHttpCall()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();
        var (_, orgId, installId) = await SeedAsync(db);
        var prCheck = MakePrCheck(orgId, installId);
        db.PrChecks.Add(prCheck);
        await db.SaveChangesAsync();

        var handler = new FakeHttpMessageHandler(HttpStatusCode.Created, """{"id":1}""");
        var factory = new FakeHttpClientFactory(handler);
        var clock = new FakeClock(BaseTime);
        var rateLimit = new GitLabRateLimitTracker(clock);
        // Pre-block the installation
        rateLimit.Update(installId, remaining: null, retryAfter: 60, resetAt: null);

        var options = Options.Create(new GitLabOptions());
        var poster = new GitLabCheckPoster(db, DefaultKeyProvider, rateLimit, factory, options,
            new NullAuditWriter(), clock, NullLogger<GitLabCheckPoster>.Instance);

        var result = await poster.PostAsync(prCheck.Id, CancellationToken.None);

        result.Status.Should().Be(PrCheckPostStatus.Queued);
        result.Code.Should().Be(PrCheckErrorCode.GitLabRateLimited);
        handler.RequestCount.Should().Be(0, "no HTTP call when pre-blocked");
    }
}

/// <summary>No-op audit writer for tests that don't need audit log assertions.</summary>
file sealed class NullAuditWriter : ApiTool.Backend.Audit.IAuditWriter
{
    public void Append(ApiTool.Backend.Audit.AuditEvent evt) { }
}

/// <summary>Recording audit writer for asserting audit events in tests.</summary>
file sealed class SpyAuditWriter : ApiTool.Backend.Audit.IAuditWriter
{
    private readonly List<ApiTool.Backend.Audit.AuditEvent> _events = new();

    public IReadOnlyList<ApiTool.Backend.Audit.AuditEvent> Events => _events;

    public void Append(ApiTool.Backend.Audit.AuditEvent evt) => _events.Add(evt);
}
