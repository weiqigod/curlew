// Refs plan Decision E (Push/MR stored-only), Decision J (dispatcher contract).
using System.Text.Json;
using ApiTool.Backend.GitLab.Webhooks;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Unit tests for <see cref="GitLabWebhookDispatcher"/> routing by event type.
/// Uses a tracking pipeline-hook handler to verify dispatch without DB dependencies.
/// </summary>
public sealed class GitLabWebhookDispatcherTests
{
    private static GitLabWebhookEnvelope MakeEnvelope(string eventType, string json = """{"object_kind":"pipeline","object_attributes":{"id":1,"status":"success","sha":"abc123"},"project":{"id":42}}""")
    {
        var doc = JsonDocument.Parse(json);
        return new GitLabWebhookEnvelope(
            EventUuid: Guid.NewGuid().ToString(),
            EventType: eventType,
            InstallationId: Guid.NewGuid(),
            Payload: doc);
    }

    [Fact]
    public async Task Dispatch_pipeline_hook_invokes_pipeline_handler()
    {
        var handler = new TrackingPipelineHandler();
        var dispatcher = new GitLabWebhookDispatcher(handler, NullLogger<GitLabWebhookDispatcher>.Instance);
        var envelope = MakeEnvelope("Pipeline Hook");

        await dispatcher.DispatchAsync(envelope, CancellationToken.None);

        handler.Called.Should().BeTrue("Pipeline Hook must route to pipeline handler");
    }

    [Fact]
    public async Task Dispatch_push_hook_logs_stored_only_no_handler_call()
    {
        var handler = new TrackingPipelineHandler();
        var dispatcher = new GitLabWebhookDispatcher(handler, NullLogger<GitLabWebhookDispatcher>.Instance);
        var envelope = MakeEnvelope("Push Hook", """{"object_kind":"push"}""");

        // Must not throw
        await dispatcher.DispatchAsync(envelope, CancellationToken.None);

        handler.Called.Should().BeFalse("Push Hook must not invoke the pipeline handler");
    }

    [Fact]
    public async Task Dispatch_merge_request_hook_logs_stored_only_no_handler_call()
    {
        var handler = new TrackingPipelineHandler();
        var dispatcher = new GitLabWebhookDispatcher(handler, NullLogger<GitLabWebhookDispatcher>.Instance);
        var envelope = MakeEnvelope("Merge Request Hook", """{"object_kind":"merge_request"}""");

        await dispatcher.DispatchAsync(envelope, CancellationToken.None);

        handler.Called.Should().BeFalse("Merge Request Hook must not invoke the pipeline handler");
    }

    [Fact]
    public async Task Dispatch_unknown_event_logs_unhandled_returns_success()
    {
        var handler = new TrackingPipelineHandler();
        var dispatcher = new GitLabWebhookDispatcher(handler, NullLogger<GitLabWebhookDispatcher>.Instance);
        var envelope = MakeEnvelope("Note Hook", """{"object_kind":"note"}""");

        // Must not throw
        await dispatcher.DispatchAsync(envelope, CancellationToken.None);

        handler.Called.Should().BeFalse();
    }

    // ── Test doubles ─────────────────────────────────────────────────────────

    private sealed class TrackingPipelineHandler : GitLabPipelineHookHandler
    {
        public TrackingPipelineHandler()
            : base(null!, NullLogger<GitLabPipelineHookHandler>.Instance)
        {
        }

        public bool Called { get; private set; }

        public override Task HandleAsync(GitLabPipelineHookPayload payload, Guid installationId, CancellationToken ct)
        {
            Called = true;
            return Task.CompletedTask;
        }
    }
}
