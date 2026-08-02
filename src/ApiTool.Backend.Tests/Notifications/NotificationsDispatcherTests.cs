using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Notifications;

/// <summary>Tests for <see cref="NotificationsDispatcher"/>.</summary>
public sealed class NotificationsDispatcherTests
{
    // ── Test helpers ──────────────────────────────────────────────────────────

    private static async Task<(
        TestDbScope scope,
        AppDbContext db,
        NotificationsDispatcher dispatcher,
        FakeSlackWebhookPoster slackPoster,
        FakeSmtpSender smtpSender,
        Guid userId,
        Guid orgId)> BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"owner-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"disp-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var slackPoster = new FakeSlackWebhookPoster();
        var smtpSender = new FakeSmtpSender();

        // Options with zero delays so retries happen instantly
        var options = Options.Create(new NotificationsDispatcherOptions
        {
            RetryDelays = [TimeSpan.Zero, TimeSpan.Zero],
        });

        // Use the same db instance directly via a FakeScopeFactory so we share state
        var notifService = new NotificationsService(db, TimeProvider.System);
        var scopeFactory = new FakeScopeFactory(db, notifService);

        var dispatcher = new NotificationsDispatcher(
            scopeFactory,
            slackPoster,
            smtpSender,
            TimeProvider.System,
            options,
            NullLogger<NotificationsDispatcher>.Instance);

        return (scope, db, dispatcher, slackPoster, smtpSender, userId, orgId);
    }

    private static Guid SeedResult(AppDbContext db, Guid orgId, Guid userId, int failCount = 0)
    {
        var resultId = Guid.NewGuid();
        db.Results.Add(new Result
        {
            Id = resultId,
            OrgId = orgId,
            UploadedBy = userId,
            CollectionName = "smoke-tests",
            RunAt = DateTime.UtcNow,
            DurationMs = 1000,
            PassCount = failCount == 0 ? 2 : 1,
            FailCount = failCount,
            SkippedCount = 0,
            CreatedAt = DateTime.UtcNow,
        });
        db.SaveChanges();
        return resultId;
    }

    private static Guid SeedSlackRule(AppDbContext db, Guid orgId, Guid userId,
        string webhookUrl = "https://hooks.slack.test/xyz")
    {
        var ruleId = Guid.NewGuid();
        db.NotificationRules.Add(new NotificationRule
        {
            Id = ruleId,
            OrgId = orgId,
            Channel = NotificationChannel.Slack,
            Target = webhookUrl,
            OnEvents = "run_failed",
            CreatedBy = userId,
            CreatedAt = DateTime.UtcNow,
        });
        db.SaveChanges();
        return ruleId;
    }

    // ── Tests ────────────────────────────────────────────────────────────────

    [Fact]
    public async Task NotifyAsync_does_nothing_when_result_passes()
    {
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 0); // passing run

            await dispatcher.NotifyAsync(orgId, resultId, default);

            slack.Calls.Should().BeEmpty();
            db.ChangeTracker.Clear();
            (await db.NotificationDeliveries.CountAsync()).Should().Be(0);
        }
    }

    [Fact]
    public async Task NotifyAsync_posts_to_slack_webhook_when_rule_matches_and_run_failed()
    {
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId, "https://hooks.slack.test/post");
            var resultId = SeedResult(db, orgId, userId, failCount: 1);

            await dispatcher.NotifyAsync(orgId, resultId, default);

            slack.Calls.Should().ContainSingle(c => c.Url == "https://hooks.slack.test/post");
        }
    }

    [Fact]
    public async Task NotifyAsync_records_delivered_status_on_2xx()
    {
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var ruleId = SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 1);

            await dispatcher.NotifyAsync(orgId, resultId, default);

            db.ChangeTracker.Clear();
            var delivery = await db.NotificationDeliveries.SingleAsync();
            delivery.Status.Should().Be(NotificationDeliveryStatus.Delivered);
            delivery.ResponseCode.Should().Be(200);
            delivery.RuleId.Should().Be(ruleId);
            delivery.ResultId.Should().Be(resultId);
            delivery.OrgId.Should().Be(orgId);
        }
    }

    [Fact]
    public async Task NotifyAsync_retries_twice_on_5xx_then_records_failed()
    {
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 1);

            // Script all 3 attempts to return 500
            slack.EnqueueStatusCode(500);
            slack.EnqueueStatusCode(500);
            slack.EnqueueStatusCode(500);

            await dispatcher.NotifyAsync(orgId, resultId, default);

            // Verifies exactly 3 HTTP calls were made (initial + 2 retries)
            slack.Calls.Should().HaveCount(3, "initial attempt + 2 retries");

            db.ChangeTracker.Clear();
            var delivery = await db.NotificationDeliveries.SingleAsync();
            delivery.Status.Should().Be(NotificationDeliveryStatus.Failed);
            delivery.AttemptCount.Should().Be(3, "all 3 attempts must be recorded");
            delivery.ResponseCode.Should().Be(500);
        }
    }

    [Fact]
    public async Task NotifyAsync_records_error_message_when_all_retries_fail()
    {
        // Differentiates from NotifyAsync_retries_twice_on_5xx_then_records_failed by
        // verifying the ErrorMessage field is populated with the HTTP status text, not AttemptCount.
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 1);

            slack.EnqueueStatusCode(503);
            slack.EnqueueStatusCode(503);
            slack.EnqueueStatusCode(503);

            await dispatcher.NotifyAsync(orgId, resultId, default);

            db.ChangeTracker.Clear();
            var delivery = await db.NotificationDeliveries.SingleAsync();
            delivery.Status.Should().Be(NotificationDeliveryStatus.Failed);
            delivery.ResponseCode.Should().Be(503);
            delivery.ErrorMessage.Should().Contain("503",
                because: "error message must record the failing HTTP status code");
        }
    }

    [Fact]
    public async Task NotifyAsync_payload_contains_result_id_collection_and_failure_counts()
    {
        var (scope, db, dispatcher, slack, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 3);

            await dispatcher.NotifyAsync(orgId, resultId, default);

            slack.Calls.Should().ContainSingle();
            var payload = slack.Calls[0].Payload;
            // Verify collection name, result_id prefix, and fail_count are all present
            payload.Should().Contain("smoke-tests", because: "collection_name must be in payload");
            payload.Should().Contain($"res_{resultId:N}", because: "result_id with res_ prefix must be in payload");
            payload.Should().Contain("\"fail_count\":3", because: "fail_count value must be in payload");
        }
    }

    [Fact]
    public async Task NotifyAsync_never_throws_when_http_throws()
    {
        var (scope, db, _, _, _, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            SeedSlackRule(db, orgId, userId);
            var resultId = SeedResult(db, orgId, userId, failCount: 1);

            var throwingPoster = new ThrowingSlackPoster();
            var options = Options.Create(new NotificationsDispatcherOptions
            {
                RetryDelays = [TimeSpan.Zero, TimeSpan.Zero],
            });
            var notifService = new NotificationsService(db, TimeProvider.System);
            var scopeFactory = new FakeScopeFactory(db, notifService);
            var throwingDispatcher = new NotificationsDispatcher(
                scopeFactory,
                throwingPoster,
                new FakeSmtpSender(),
                TimeProvider.System,
                options,
                NullLogger<NotificationsDispatcher>.Instance);

            // Must not throw
            await throwingDispatcher.Invoking(d => d.NotifyAsync(orgId, resultId, default))
                .Should().NotThrowAsync();
        }
    }

    // ── Fakes ─────────────────────────────────────────────────────────────────

    /// <summary>
    /// A scope factory that resolves from a single pre-constructed <see cref="AppDbContext"/>
    /// so tests share the same SQLite in-memory database.
    /// </summary>
    private sealed class FakeScopeFactory(AppDbContext db, NotificationsService notifService) : IServiceScopeFactory
    {
        public IServiceScope CreateScope()
        {
            var services = new ServiceCollection();
            services.AddSingleton(db);
            services.AddSingleton(notifService);
            return services.BuildServiceProvider().CreateScope();
        }
    }

    /// <summary>Helper poster that always throws to test error swallowing.</summary>
    private sealed class ThrowingSlackPoster : ISlackWebhookPoster
    {
        public Task<int> PostAsync(string webhookUrl, string payload, CancellationToken ct) =>
            throw new HttpRequestException("Network unreachable");
    }
}
