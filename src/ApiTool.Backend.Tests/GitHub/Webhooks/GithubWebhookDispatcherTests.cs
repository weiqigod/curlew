// Refs docs/SPECIFICATION.md:8557 (handler wiring per event_type/action).
using System.Text.Json;
using ApiTool.Backend.GitHub.Webhooks;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubWebhookDispatcher routing.
/// </summary>
public sealed class GithubWebhookDispatcherTests
{
    private static GithubWebhookEnvelope MakeEnvelope(
        string eventType, string? action, string json = """{"installation":{"id":12345,"app_id":111},"account":{"login":"acme","type":"Organization"},"repository_selection":"selected","repositories":[]}""")
    {
        var deliveryId = Guid.NewGuid();
        var doc = JsonDocument.Parse(json);
        return new GithubWebhookEnvelope(deliveryId, eventType, action, doc);
    }

    [Fact]
    public async Task installation_created_routes_to_created_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation", "created");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.CreatedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task installation_deleted_routes_to_lifecycle_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation", "deleted");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.DeletedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task installation_suspend_routes_to_lifecycle_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation", "suspend");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.SuspendedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task installation_unsuspend_routes_to_lifecycle_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation", "unsuspend");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.UnsuspendedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task installation_repositories_added_routes_to_repos_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation_repositories", "added",
            """{"installation":{"id":12345},"repositories_added":[],"repositories_removed":[]}""");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.ReposAddedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task installation_repositories_removed_routes_to_repos_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("installation_repositories", "removed",
            """{"installation":{"id":12345},"repositories_added":[],"repositories_removed":[]}""");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.ReposRemovedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task check_run_rerequested_routes_to_check_run_handler()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("check_run", "rerequested",
            """{"check_run":{"external_id":"","head_sha":"abc"}}""");

        await dispatcher.DispatchAsync(env, CancellationToken.None);

        tracker.CheckRunRerequestedCalled.Should().BeTrue();
    }

    [Fact]
    public async Task unknown_event_logs_and_returns()
    {
        var tracker = new RoutingTracker();
        var dispatcher = tracker.CreateDispatcher();
        var env = MakeEnvelope("marketplace_purchase", "purchased");

        // Should not throw
        await dispatcher.DispatchAsync(env, CancellationToken.None);

        // No handler should have been called
        tracker.CreatedCalled.Should().BeFalse();
        tracker.DeletedCalled.Should().BeFalse();
    }

    // ── Test doubles ─────────────────────────────────────────────────────────

    private sealed class RoutingTracker
    {
        public bool CreatedCalled { get; private set; }
        public bool DeletedCalled { get; private set; }
        public bool SuspendedCalled { get; private set; }
        public bool UnsuspendedCalled { get; private set; }
        public bool ReposAddedCalled { get; private set; }
        public bool ReposRemovedCalled { get; private set; }
        public bool CheckRunRerequestedCalled { get; private set; }

        public IGithubWebhookDispatcher CreateDispatcher()
        {
            var created = new TrackingCreatedHandler(this);
            var repos = new TrackingReposHandler(this);
            var lifecycle = new TrackingLifecycleHandler(this);
            var checkRun = new TrackingCheckRunHandler(this);
            return new GithubWebhookDispatcher(
                created, repos, lifecycle, checkRun,
                NullLogger<GithubWebhookDispatcher>.Instance);
        }

        private sealed class TrackingCreatedHandler(RoutingTracker t) : GithubInstallationCreatedHandler(null!)
        {
            public override Task HandleAsync(GithubInstallationCreatedPayload payload, CancellationToken ct)
            {
                t.CreatedCalled = true;
                return Task.CompletedTask;
            }
        }

        private sealed class TrackingReposHandler(RoutingTracker t) : GithubInstallationRepositoriesHandler(null!)
        {
            public override Task HandleAddedAsync(GithubInstallationRepositoriesPayload payload, CancellationToken ct)
            {
                t.ReposAddedCalled = true;
                return Task.CompletedTask;
            }
            public override Task HandleRemovedAsync(GithubInstallationRepositoriesPayload payload, CancellationToken ct)
            {
                t.ReposRemovedCalled = true;
                return Task.CompletedTask;
            }
        }

        private sealed class TrackingLifecycleHandler(RoutingTracker t) : GithubInstallationLifecycleHandler(null!)
        {
            public override Task HandleDeletedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct)
            {
                t.DeletedCalled = true;
                return Task.CompletedTask;
            }
            public override Task HandleSuspendedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct)
            {
                t.SuspendedCalled = true;
                return Task.CompletedTask;
            }
            public override Task HandleUnsuspendedAsync(GithubInstallationLifecyclePayload p, CancellationToken ct)
            {
                t.UnsuspendedCalled = true;
                return Task.CompletedTask;
            }
        }

        private sealed class TrackingCheckRunHandler(RoutingTracker t) : GithubCheckRunHandler(null!, null!, null!)
        {
            public override Task HandleRerequestedAsync(GithubCheckRunRerequestedPayload payload, CancellationToken ct)
            {
                t.CheckRunRerequestedCalled = true;
                return Task.CompletedTask;
            }
        }
    }
}
