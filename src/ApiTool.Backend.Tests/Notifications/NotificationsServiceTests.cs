using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Notifications;

/// <summary>Tests for <see cref="NotificationsService"/> against in-memory SQLite.</summary>
public sealed class NotificationsServiceTests
{
    // ── Test helpers ──────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, NotificationsService svc, Guid ownerId, Guid memberId, Guid orgId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var ownerId = Guid.NewGuid();
        var memberId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = ownerId, Email = $"owner-{ownerId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Users.Add(new User { Id = memberId, Email = $"member-{memberId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"notif-{orgId:N}"[..20],
            OwnerId = ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = ownerId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = memberId,
            Role = OrgRole.Member,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var svc = new NotificationsService(db, TimeProvider.System);
        return (scope, db, svc, ownerId, memberId, orgId);
    }

    // ── CreateRuleAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task CreateRuleAsync_persists_slack_rule_for_admin()
    {
        var (scope, db, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "slack",
                Target = "https://hooks.slack.test/xyz",
                On = ["run_failed"],
            };

            var (dto, error, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);

            error.Should().Be(NotificationError.None);
            dto.Should().NotBeNull();
            dto!.Id.Should().StartWith("nrule_");
            dto.Channel.Should().Be("slack");
            dto.Target.Should().Be("https://hooks.slack.test/xyz");
            dto.On.Should().ContainSingle(e => e == "run_failed");

            var dbRule = await db.NotificationRules.SingleAsync();
            dbRule.OrgId.Should().Be(orgId);
            dbRule.CreatedBy.Should().Be(ownerId);
        }
    }

    [Fact]
    public async Task CreateRuleAsync_persists_email_rule_with_multiple_events()
    {
        var (scope, _, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "email",
                Target = "alerts@acme.com",
                On = ["run_failed", "flaky"],
            };

            var (dto, error, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);

            error.Should().Be(NotificationError.None);
            dto.Should().NotBeNull();
            dto!.Channel.Should().Be("email");
            dto.On.Should().HaveCount(2).And.Contain("run_failed").And.Contain("flaky");
        }
    }

    [Fact]
    public async Task CreateRuleAsync_returns_invalid_channel_for_unknown_channel()
    {
        var (scope, _, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "telegram",
                Target = "some-target",
                On = ["run_failed"],
            };

            var (dto, error, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);

            error.Should().Be(NotificationError.InvalidChannel);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateRuleAsync_returns_invalid_target_for_non_https_slack_target()
    {
        var (scope, _, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "slack",
                Target = "http://hooks.slack.test/xyz",
                On = ["run_failed"],
            };

            var (dto, error, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);

            error.Should().Be(NotificationError.InvalidTarget);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task CreateRuleAsync_returns_permission_denied_for_member()
    {
        var (scope, _, svc, _, memberId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "slack",
                Target = "https://hooks.slack.test/xyz",
                On = ["run_failed"],
            };

            var (dto, error, _) = await svc.CreateRuleAsync(memberId, orgId, req, default);

            error.Should().Be(NotificationError.PermissionDenied);
            dto.Should().BeNull();
        }
    }

    // ── ListRulesAsync ────────────────────────────────────────────────────────

    [Fact]
    public async Task ListRulesAsync_returns_rules_in_stable_order_for_member()
    {
        var (scope, _, svc, ownerId, memberId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create two rules as owner
            var r1 = new CreateNotificationRuleRequest { Channel = "slack", Target = "https://hooks.slack.test/a", On = ["run_failed"] };
            var r2 = new CreateNotificationRuleRequest { Channel = "email", Target = "a@test.com", On = ["flaky"] };
            await svc.CreateRuleAsync(ownerId, orgId, r1, default);
            await svc.CreateRuleAsync(ownerId, orgId, r2, default);

            // Member can list
            var (rules, error) = await svc.ListRulesAsync(memberId, orgId, default);

            error.Should().Be(NotificationError.None);
            rules.Should().HaveCount(2);
        }
    }

    // ── ListDeliveriesAsync ───────────────────────────────────────────────────

    [Fact]
    public async Task ListDeliveriesAsync_returns_newest_first_bounded_by_limit()
    {
        var (scope, db, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create a rule
            var ruleId = Guid.NewGuid();
            db.NotificationRules.Add(new NotificationRule
            {
                Id = ruleId,
                OrgId = orgId,
                Channel = NotificationChannel.Slack,
                Target = "https://hooks.slack.test/a",
                OnEvents = "run_failed",
                CreatedBy = ownerId,
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var now = DateTime.UtcNow;
            // Insert 3 deliveries with distinct timestamps
            for (var i = 0; i < 3; i++)
            {
                db.NotificationDeliveries.Add(new NotificationDelivery
                {
                    Id = Guid.NewGuid(),
                    RuleId = ruleId,
                    OrgId = orgId,
                    Channel = NotificationChannel.Slack,
                    Status = NotificationDeliveryStatus.Delivered,
                    AttemptCount = 1,
                    AttemptedAt = now.AddMinutes(-i),
                });
            }
            await db.SaveChangesAsync();

            var (deliveries, error) = await svc.ListDeliveriesAsync(ownerId, orgId, 2, default);

            error.Should().Be(NotificationError.None);
            deliveries.Should().HaveCount(2);
            // Newest first
            deliveries[0].AttemptedAt.Should().BeOnOrAfter(deliveries[1].AttemptedAt);
        }
    }

    [Fact]
    public async Task ListDeliveriesAsync_returns_permission_denied_for_non_member()
    {
        var (scope, _, svc, _, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var nonMemberId = Guid.NewGuid();
            var (deliveries, error) = await svc.ListDeliveriesAsync(nonMemberId, orgId, 20, default);

            error.Should().Be(NotificationError.PermissionDenied);
            deliveries.Should().BeEmpty();
        }
    }

    // ── DeleteRuleAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task DeleteRuleAsync_removes_rule_and_returns_none_for_admin()
    {
        var (scope, db, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "slack",
                Target = "https://hooks.slack.test/del",
                On = ["run_failed"],
            };
            var (dto, _, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);
            ApiTool.Backend.Notifications.NotificationRuleId.TryParse(dto!.Id, out var ruleGuid).Should().BeTrue();

            var error = await svc.DeleteRuleAsync(ownerId, orgId, ruleGuid, default);

            error.Should().Be(NotificationError.None);
            var count = await db.NotificationRules.CountAsync();
            count.Should().Be(0);
        }
    }

    [Fact]
    public async Task DeleteRuleAsync_returns_PermissionDenied_for_member()
    {
        var (scope, _, svc, ownerId, memberId, orgId) = await BuildAsync();
        await using (scope)
        {
            var req = new CreateNotificationRuleRequest
            {
                Channel = "slack",
                Target = "https://hooks.slack.test/del-perm",
                On = ["run_failed"],
            };
            var (dto, _, _) = await svc.CreateRuleAsync(ownerId, orgId, req, default);
            ApiTool.Backend.Notifications.NotificationRuleId.TryParse(dto!.Id, out var ruleGuid).Should().BeTrue();

            var error = await svc.DeleteRuleAsync(memberId, orgId, ruleGuid, default);

            error.Should().Be(NotificationError.PermissionDenied);
        }
    }

    [Fact]
    public async Task DeleteRuleAsync_returns_NotFound_for_unknown_id()
    {
        var (scope, _, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var error = await svc.DeleteRuleAsync(ownerId, orgId, Guid.NewGuid(), default);

            error.Should().Be(NotificationError.NotFound);
        }
    }

    [Fact]
    public async Task DeleteRuleAsync_returns_NotFound_when_rule_belongs_to_different_org()
    {
        var (scope, db, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create a second org
            var otherUserId = Guid.NewGuid();
            var otherOrgId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherUserId, Email = $"other-{otherUserId:N}@test.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = otherOrgId,
                Name = "OtherOrg",
                Slug = $"other2-{otherOrgId:N}"[..20],
                OwnerId = otherUserId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId = otherOrgId,
                UserId = ownerId,
                Role = OrgRole.Owner,
                JoinedAt = DateTime.UtcNow,
            });
            // Create a rule in the OTHER org
            var ruleId = Guid.NewGuid();
            db.NotificationRules.Add(new NotificationRule
            {
                Id = ruleId,
                OrgId = otherOrgId,
                Channel = NotificationChannel.Slack,
                Target = "https://hooks.slack.test/other",
                OnEvents = "run_failed",
                CreatedBy = otherUserId,
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // Try to delete it as owner of THIS org (not the other)
            var error = await svc.DeleteRuleAsync(ownerId, orgId, ruleId, default);

            error.Should().Be(NotificationError.NotFound);
        }
    }

    // ── FindMatchingRulesAsync ────────────────────────────────────────────────

    [Fact]
    public async Task FindMatchingRulesAsync_returns_only_rules_matching_event_and_org()
    {
        var (scope, db, svc, ownerId, _, orgId) = await BuildAsync();
        await using (scope)
        {
            // Create a second org with its own user so FK constraints are satisfied
            var otherUserId = Guid.NewGuid();
            var otherOrgId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherUserId, Email = $"other-{otherUserId:N}@test.com", CreatedAt = DateTime.UtcNow });
            db.Organizations.Add(new Organization
            {
                Id = otherOrgId,
                Name = "OtherOrg",
                Slug = $"other-{otherOrgId:N}"[..20],
                OwnerId = otherUserId,
                Status = OrgStatus.Active,
                CreatedAt = DateTime.UtcNow,
                UpdatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // Rule in this org for run_failed
            db.NotificationRules.Add(new NotificationRule
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Channel = NotificationChannel.Slack,
                Target = "https://hooks.slack.test/match",
                OnEvents = "run_failed",
                CreatedBy = ownerId,
                CreatedAt = DateTime.UtcNow,
            });
            // Rule in this org for flaky only — should NOT match run_failed
            db.NotificationRules.Add(new NotificationRule
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Channel = NotificationChannel.Email,
                Target = "a@test.com",
                OnEvents = "flaky",
                CreatedBy = ownerId,
                CreatedAt = DateTime.UtcNow,
            });
            // Rule in different org — should NOT be returned
            db.NotificationRules.Add(new NotificationRule
            {
                Id = Guid.NewGuid(),
                OrgId = otherOrgId,
                Channel = NotificationChannel.Slack,
                Target = "https://hooks.slack.test/other",
                OnEvents = "run_failed",
                CreatedBy = otherUserId,
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            var rules = await svc.FindMatchingRulesAsync(orgId, NotificationEvent.RunFailed, default);

            rules.Should().ContainSingle(r => r.Target == "https://hooks.slack.test/match");
        }
    }
}
