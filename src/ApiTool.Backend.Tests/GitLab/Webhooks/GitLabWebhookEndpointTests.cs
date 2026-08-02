// Refs docs/SPECIFICATION.md:9257-9263 (constant-time token verification, idempotency).
// Refs M16-015 task YAML behaviors #1-6.
using System.Net;
using System.Text;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// HTTP-level integration tests for <c>POST /webhooks/gitlab</c> covering:
/// token verification, idempotency, source-IP quarantine, dispatcher invocation,
/// and event-row quarantine on repeated dispatcher failures.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class GitLabWebhookEndpointTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public GitLabWebhookEndpointTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private const string PipelineHookBody = """
    {
      "object_kind": "pipeline",
      "object_attributes": { "id": 4242, "status": "success", "sha": "abc123def4567890abc123def4567890abc12345", "ref": "main" },
      "project": { "id": 31337, "path_with_namespace": "test-group/test-project" }
    }
    """;

    // ── Helpers ───────────────────────────────────────────────────────────────

    private HttpClient CreateClient(Action<IServiceCollection>? configureServices = null)
    {
        return _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureServices(services =>
            {
                // Default: noop dispatcher so ingest tests don't depend on installation DB state.
                services.RemoveAll<IGitLabWebhookDispatcher>();
                services.AddScoped<IGitLabWebhookDispatcher, NoopGitLabWebhookDispatcher>();

                // Each test gets a fresh source tracker to avoid cross-test quarantine.
                services.RemoveAll<GitLabWebhookSourceTracker>();
                services.AddSingleton<GitLabWebhookSourceTracker>();

                configureServices?.Invoke(services);
            });
        }).CreateClient();
    }

    /// <summary>Seeds an installation with the given plaintext secret (or a generated one).</summary>
    private async Task<(Guid installationId, string plaintextSecret)> SeedInstallation(
        string? customSecret = null)
    {
        var secret = customSecret ?? "whsec_gl_" + Guid.NewGuid().ToString("N")[..8];
        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var kp = scope.ServiceProvider.GetRequiredService<IGitLabKeyProvider>();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"u{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "WebhookOrg", Slug = $"wh{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });

        var instId = Guid.NewGuid();
        var secretBytes = Encoding.UTF8.GetBytes(secret);
        var enc = await kp.EncryptAsync(secretBytes);
        db.GitLabInstallations.Add(new GitLabInstallation
        {
            Id = instId, OrgId = orgId, ProjectId = 31337L, ProjectPath = "test-group/test-project",
            AccessTokenCiphertext = [1], AccessTokenKid = "test-kid",
            WebhookSecretCiphertext = enc.Ciphertext,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        return (instId, secret);
    }

    private static HttpRequestMessage BuildRequest(
        string token, string eventUuid, string eventType, string body = PipelineHookBody)
    {
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/gitlab")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Gitlab-Token", token);
        request.Headers.Add("X-Gitlab-Event-UUID", eventUuid);
        request.Headers.Add("X-Gitlab-Event", eventType);
        return request;
    }

    // ── Tests ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task Valid_token_returns_200_and_inserts_row()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Pipeline Hook");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().NotBeNull("a row must be inserted on first delivery");
        row!.FailureCount.Should().Be(0);
    }

    [Fact]
    public async Task Invalid_token_returns_401_no_row_inserted()
    {
        await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest("wrong-secret", eventUuid, "Pipeline Hook");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().BeNull("401 must not insert a row");
    }

    [Fact]
    public async Task Missing_token_header_returns_401()
    {
        await SeedInstallation();
        var client = CreateClient();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/gitlab")
        {
            Content = new StringContent(PipelineHookBody, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Gitlab-Event-UUID", Guid.NewGuid().ToString());
        request.Headers.Add("X-Gitlab-Event", "Pipeline Hook");
        // No X-Gitlab-Token

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Missing_event_uuid_returns_400()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/gitlab")
        {
            Content = new StringContent(PipelineHookBody, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Gitlab-Token", secret);
        request.Headers.Add("X-Gitlab-Event", "Pipeline Hook");
        // No X-Gitlab-Event-UUID

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Missing_event_type_returns_400()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/gitlab")
        {
            Content = new StringContent(PipelineHookBody, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Gitlab-Token", secret);
        request.Headers.Add("X-Gitlab-Event-UUID", Guid.NewGuid().ToString());
        // No X-Gitlab-Event

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task Duplicate_event_uuid_returns_200_no_second_row()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var req1 = BuildRequest(secret, eventUuid, "Pipeline Hook");
        var req2 = BuildRequest(secret, eventUuid, "Pipeline Hook");

        var resp1 = await client.SendAsync(req1);
        var resp2 = await client.SendAsync(req2);

        resp1.StatusCode.Should().Be(HttpStatusCode.OK);
        resp2.StatusCode.Should().Be(HttpStatusCode.OK, "duplicate event must return 200 idempotent no-op");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var count = await db.GitLabWebhookEvents.CountAsync(x => x.EventUuid == eventUuid);
        count.Should().Be(1, "only one row must exist for a duplicate event_uuid");
    }

    [Fact]
    public async Task Push_hook_stored_only_returns_200()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Push Hook", """{"object_kind":"push"}""");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Merge_request_hook_stored_only_returns_200()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Merge Request Hook", """{"object_kind":"merge_request"}""");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]
    public async Task Handler_exception_records_failure_returns_500()
    {
        var (_, secret) = await SeedInstallation();
        var throwing = new ThrowingGitLabWebhookDispatcher(throwUntilAttempt: 1);
        var client = CreateClient(services =>
        {
            services.RemoveAll<IGitLabWebhookDispatcher>();
            services.AddScoped<IGitLabWebhookDispatcher>(_ => throwing);
        });

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Pipeline Hook");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.InternalServerError);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().NotBeNull();
        row!.FailureCount.Should().Be(1);
    }

    [Fact]
    public async Task Fifth_handler_failure_quarantines_event_row_returns_200()
    {
        var (_, secret) = await SeedInstallation();
        var throwing = new ThrowingGitLabWebhookDispatcher(throwUntilAttempt: int.MaxValue);
        var client = CreateClient(services =>
        {
            services.RemoveAll<IGitLabWebhookDispatcher>();
            services.AddScoped<IGitLabWebhookDispatcher>(_ => throwing);
        });

        var eventUuid = Guid.NewGuid().ToString();
        HttpStatusCode? lastStatus = null;
        for (var i = 0; i < GitLabWebhookStore.QuarantineThreshold; i++)
        {
            var request = BuildRequest(secret, eventUuid, "Pipeline Hook");
            var response = await client.SendAsync(request);
            lastStatus = response.StatusCode;
        }

        lastStatus.Should().Be(HttpStatusCode.OK, "5th failure should quarantine and return 200 to break retry storm");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().NotBeNull();
        row!.QuarantinedAt.Should().NotBeNull();
        row.FailureCount.Should().Be(GitLabWebhookStore.QuarantineThreshold);
    }

    [Fact]
    public async Task Fifth_verification_failure_quarantines_source_ip_returns_429()
    {
        await SeedInstallation();
        var client = CreateClient();

        HttpStatusCode? lastStatus = null;
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
        {
            var request = BuildRequest("wrong-secret-" + i, Guid.NewGuid().ToString(), "Pipeline Hook");
            var response = await client.SendAsync(request);
            lastStatus = response.StatusCode;
        }

        lastStatus.Should().Be(HttpStatusCode.TooManyRequests, "5th bad token from same IP should return 429");
    }

    [Fact]
    public async Task Quarantined_source_ip_returns_429_before_verification()
    {
        await SeedInstallation();
        var client = CreateClient();

        // Exhaust the threshold with wrong tokens
        for (var i = 0; i < GitLabWebhookSourceTracker.QuarantineThreshold; i++)
        {
            await client.SendAsync(
                BuildRequest("wrong-secret-" + i, Guid.NewGuid().ToString(), "Pipeline Hook"));
        }

        // Next request (even with correct secret) should return 429 immediately
        var (_, correctSecret) = await SeedInstallation();
        var nextRequest = BuildRequest(correctSecret, Guid.NewGuid().ToString(), "Pipeline Hook");
        var nextResponse = await client.SendAsync(nextRequest);
        nextResponse.StatusCode.Should().Be(HttpStatusCode.TooManyRequests);
    }

    [Fact]
    public async Task Successful_processing_marks_event_status_processed()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Pipeline Hook");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK);

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().NotBeNull();
        row!.ProcessedAt.Should().NotBeNull("processed_at must be set on success");
    }

    [Fact]
    public async Task Invalid_json_after_valid_token_returns_400()
    {
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/gitlab")
        {
            Content = new StringContent("not-json", Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("X-Gitlab-Token", secret);
        request.Headers.Add("X-Gitlab-Event-UUID", Guid.NewGuid().ToString());
        request.Headers.Add("X-Gitlab-Event", "Pipeline Hook");

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.BadRequest);
    }

    [Fact]
    public async Task No_installations_configured_returns_401()
    {
        // Don't seed any installation
        var client = CreateClient();

        var request = BuildRequest("any-secret", Guid.NewGuid().ToString(), "Pipeline Hook");
        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.Unauthorized);
    }

    [Fact]
    public async Task Observable_pipeline_hook_from_fixture_succeeds()
    {
        // Verifies the observable from task YAML: load fixture, post it, check 200 and row in DB.
        var (_, secret) = await SeedInstallation();
        var client = CreateClient();

        var fixturePath = Path.Combine(
            AppContext.BaseDirectory, "testdata", "gitlab-pipeline-hook.json");
        var fixtureBody = await File.ReadAllTextAsync(fixturePath);

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Pipeline Hook", fixtureBody);
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK, "fixture pipeline hook must be accepted");

        using var scope = _factory.Services.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var row = await db.GitLabWebhookEvents.FirstOrDefaultAsync(x => x.EventUuid == eventUuid);
        row.Should().NotBeNull("a row must be inserted for the pipeline hook fixture");
        row!.FailureCount.Should().Be(0);
        row.ProcessedAt.Should().NotBeNull("processed_at must be set on success");
    }

    /// <summary>
    /// Behavior #4: Given X-Gitlab-Event = 'Pipeline Hook', when the handler runs,
    /// then the dispatcher is actually invoked with the correct envelope.
    /// Covers the wire-to-end path: endpoint → dispatcher → recorded call.
    /// </summary>
    [Fact]
    public async Task Pipeline_hook_invokes_dispatcher_with_correct_envelope()
    {
        var (_, secret) = await SeedInstallation();
        var recording = new RecordingGitLabWebhookDispatcher();
        var client = CreateClient(services =>
        {
            services.RemoveAll<IGitLabWebhookDispatcher>();
            services.AddScoped<IGitLabWebhookDispatcher>(_ => recording);
        });

        var eventUuid = Guid.NewGuid().ToString();
        var request = BuildRequest(secret, eventUuid, "Pipeline Hook");
        var response = await client.SendAsync(request);

        response.StatusCode.Should().Be(HttpStatusCode.OK);
        recording.Calls.Should().HaveCount(1, "dispatcher must be invoked exactly once for a Pipeline Hook");
        recording.Calls[0].EventType.Should().Be("Pipeline Hook");
        recording.Calls[0].EventUuid.Should().Be(eventUuid);
    }
}

/// <summary>No-op dispatcher for ingest pipeline tests.</summary>
internal sealed class NoopGitLabWebhookDispatcher : IGitLabWebhookDispatcher
{
    public Task DispatchAsync(GitLabWebhookEnvelope envelope, CancellationToken ct) => Task.CompletedTask;
}

/// <summary>Throwing dispatcher for quarantine/failure tests.</summary>
internal sealed class ThrowingGitLabWebhookDispatcher(int throwUntilAttempt) : IGitLabWebhookDispatcher
{
    private int _callCount;

    public Task DispatchAsync(GitLabWebhookEnvelope envelope, CancellationToken ct)
    {
        var n = Interlocked.Increment(ref _callCount);
        if (n <= throwUntilAttempt)
            throw new InvalidOperationException($"Simulated handler failure #{n}");
        return Task.CompletedTask;
    }
}

/// <summary>
/// Recording dispatcher that captures every <see cref="GitLabWebhookEnvelope"/> it receives.
/// Used to verify endpoint-level dispatcher invocation without coupling to business logic.
/// </summary>
internal sealed class RecordingGitLabWebhookDispatcher : IGitLabWebhookDispatcher
{
    private readonly List<GitLabWebhookEnvelope> _calls = [];

    public IReadOnlyList<GitLabWebhookEnvelope> Calls => _calls;

    public Task DispatchAsync(GitLabWebhookEnvelope envelope, CancellationToken ct)
    {
        _calls.Add(envelope);
        return Task.CompletedTask;
    }
}
