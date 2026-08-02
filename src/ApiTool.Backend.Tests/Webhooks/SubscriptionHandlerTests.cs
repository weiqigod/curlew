// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Unit tests for StripeSubscriptionHandler behaviors.
/// Each test seeds a SQLite in-memory DB and drives the handler directly.
/// </summary>
public sealed class SubscriptionHandlerTests
{
    // ── Test fixture helpers ────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, Guid userId, Guid orgId)>
        SeedOrgAsync(string ownerEmail = "owner@example.com")
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = ownerEmail, CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Acme", Slug = $"acme-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        return (scope, db, userId, orgId);
    }

    private static Subscription SeedSubscription(AppDbContext db, Guid orgId,
        string stripeCustomerId = "cus_test_owner",
        string? stripeSubscriptionId = "sub_test_1",
        SubscriptionStatus status = SubscriptionStatus.Active,
        SubscriptionTier tier = SubscriptionTier.Team)
    {
        var sub = new Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = tier, Status = status,
            Interval = "month", SeatCount = 3, SeatLimit = 3,
            StripeCustomerId = stripeCustomerId,
            StripeSubscriptionId = stripeSubscriptionId,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        };
        db.Subscriptions.Add(sub);
        return sub;
    }

    private static Stripe.Subscription MakeStripeSub(
        string id = "sub_test_1",
        string customerId = "cus_test_owner",
        string status = "active",
        string priceId = "price_test_team_monthly",
        long quantity = 3)
    {
        return new Stripe.Subscription
        {
            Id = id,
            CustomerId = customerId,
            Status = status,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CancelAtPeriodEnd = false,
            Items = new Stripe.StripeList<Stripe.SubscriptionItem>
            {
                Data =
                [
                    new Stripe.SubscriptionItem
                    {
                        Price = new Stripe.Price { Id = priceId },
                        Quantity = quantity,
                    },
                ],
            },
        };
    }

    private static StripeSubscriptionHandler BuildHandler(
        AppDbContext db,
        IStripeGateway gateway,
        IEmailQueue? emailQueue = null)
    {
        emailQueue ??= new RecordingEmailQueue();
        return new StripeSubscriptionHandler(
            db, gateway, emailQueue, TimeProvider.System,
            NullLogger<StripeSubscriptionHandler>.Instance);
    }

    // ── Test 1: Created_event_refetches_and_upserts_subscription_row ────────

    [Fact]
    public async Task Created_event_refetches_and_upserts_subscription_row()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, stripeSubscriptionId: null); // no sub_id yet (pre-checkout)
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub());

            var handler = BuildHandler(db, gateway);
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeSubscriptionId.Should().Be("sub_test_1");
            row.Status.Should().Be(SubscriptionStatus.Active);
            row.Tier.Should().Be(SubscriptionTier.Team);
        }
    }

    // ── Test 2: Created_event_uses_stripe_state_not_event_payload ───────────

    [Fact]
    public async Task Created_event_uses_stripe_state_not_event_payload()
    {
        // The "event payload" is never passed to the handler — only the sub_id is.
        // This proves the handler always re-fetches from the gateway (Stripe is the source of truth).
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, stripeSubscriptionId: null);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            // Stripe says the sub is "active" with 5 seats — not 3 (the event payload might say 3).
            gateway.SetSubscriptionForTest(MakeStripeSub(quantity: 5, status: "active"));

            var handler = BuildHandler(db, gateway);
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            // Handler must have used the live Stripe state (5 seats), not any "event payload".
            row.SeatCount.Should().Be(5);
        }
    }

    // ── Test 3: Updated_arriving_before_created_writes_stripe_current_state ─

    [Fact]
    public async Task Updated_arriving_before_created_writes_stripe_current_state()
    {
        // Simulate out-of-order: row exists (from checkout) with customer_id but no sub_id.
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId,
                stripeCustomerId: "cus_test_owner",
                stripeSubscriptionId: null); // no sub_id
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "trialing"));

            var handler = BuildHandler(db, gateway);
            // updated event arrives before created — still correct (re-fetch wins)
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: false, default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.StripeSubscriptionId.Should().Be("sub_test_1");
            // Stripe status trialing → local Active
            row.Status.Should().Be(SubscriptionStatus.Active);
        }
    }

    // ── Test 4: Second_arrival_of_same_event_is_a_noop ──────────────────────

    [Fact]
    public async Task Second_arrival_of_same_event_is_a_noop()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub());

            var handler = BuildHandler(db, gateway);

            // First arrival
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);
            // Second arrival — should be a pure upsert, no error, same final state
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: false, default);

            db.ChangeTracker.Clear();
            // Should still have exactly one subscription row
            var rows = await db.Subscriptions.Where(s => s.OrgId == orgId).ToListAsync();
            rows.Should().HaveCount(1);
            rows[0].StripeSubscriptionId.Should().Be("sub_test_1");
        }
    }

    // ── Test 5: Deleted_event_marks_canceled_clears_subscription_id ─────────

    [Fact]
    public async Task Deleted_event_marks_canceled_clears_subscription_id()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            // deleted sub: Stripe returns it (still accessible briefly after deletion)
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            var handler = BuildHandler(db, gateway);
            await handler.HandleDeletedAsync("sub_test_1", default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.Status.Should().Be(SubscriptionStatus.Canceled);
            row.StripeSubscriptionId.Should().BeNull(); // cleared per behavior #3
        }
    }

    // ── Test 6: Deleted_event_enqueues_billing_subscription_canceled_email ──

    [Fact]
    public async Task Deleted_event_enqueues_billing_subscription_canceled_email()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync("billing@acme.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandleDeletedAsync("sub_test_1", default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.TemplateSlug.Should().Be("billing_subscription_canceled");
            msg.To.Should().Be("billing@acme.com");
        }
    }

    // ── Test 7: Deleted_event_email_uses_owner_first_name_from_email_local_part

    [Fact]
    public async Task Deleted_event_email_uses_owner_first_name_from_email_local_part()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync("alice@example.com");
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);
            await handler.HandleDeletedAsync("sub_test_1", default);

            emailQueue.Messages.Should().HaveCount(1);
            var msg = emailQueue.Messages[0];
            msg.Variables["first_name"].Should().Be("alice");
        }
    }

    // ── Test 8: Deleted_event_with_canceled_subscription_falls_back_to_free_tier

    [Fact]
    public async Task Deleted_event_with_canceled_subscription_falls_back_to_free_tier()
    {
        // After HandleDeletedAsync, GetForOrgAsync should return "free" because status=Canceled.
        var (scope, db, userId, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            var handler = BuildHandler(db, gateway);
            await handler.HandleDeletedAsync("sub_test_1", default);

            db.ChangeTracker.Clear();
            var auditWriter = new AuditWriter(db, new AuditContext(), TimeProvider.System);
            var svc = new SubscriptionsService(db, gateway, TimeProvider.System, auditWriter);
            var (_, tier) = await svc.GetForOrgAsync(userId, orgId, default);
            tier.Should().Be("free");
        }
    }

    // ── Test N: Created_event_for_valid_subscription_with_no_local_row_is_noop

    [Fact]
    public async Task Created_event_for_valid_subscription_with_no_local_row_is_noop()
    {
        // Gateway returns a non-null Stripe.Subscription but there is no local row
        // matching by StripeSubscriptionId or by (StripeCustomerId + null sub_id).
        // The handler must log and return without throwing, and must not create any row.
        var (scope, db, _, _) = await SeedOrgAsync();
        await using (scope)
        {
            // No subscription row seeded at all.
            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub("sub_orphan", "cus_orphan"));

            var handler = BuildHandler(db, gateway);

            // Should complete without throwing — no row to upsert.
            var act = async () => await handler.HandleCreatedOrUpdatedAsync("sub_orphan", isCreated: true, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.Subscriptions.CountAsync()).Should().Be(0);
        }
    }

    // ── Test ND: Deleted_event_with_no_local_row_is_noop ─────────────────────

    [Fact]
    public async Task Deleted_event_with_no_local_row_is_noop()
    {
        // No local subscription row exists for the given subscription id.
        // The handler must log and return without throwing, enqueue nothing, and create no rows.
        var (scope, db, _, _) = await SeedOrgAsync();
        await using (scope)
        {
            // No subscription row seeded at all.
            var gateway = new FakeStripeGateway();
            // Stripe can still return (or not) the subscription — both paths should be safe.

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);

            var act = async () => await handler.HandleDeletedAsync("sub_missing", default);
            await act.Should().NotThrowAsync();

            // No email enqueued.
            emailQueue.Messages.Should().BeEmpty("no local row means no owner to email");

            // No rows created.
            db.ChangeTracker.Clear();
            (await db.Subscriptions.CountAsync()).Should().Be(0);
        }
    }

    // ── Test 11: Created_event_for_404_subscription_marks_quarantined ────────

    [Fact]
    public async Task Created_event_for_404_subscription_marks_quarantined()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId); // row exists with sub_test_1
            await db.SaveChangesAsync();

            // Gateway returns null → Stripe 404
            var gateway = new FakeStripeGateway(); // sub_test_1 not scripted → returns null

            var handler = BuildHandler(db, gateway);
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);

            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.Status.Should().Be(SubscriptionStatus.Quarantined);
        }
    }

    // ── Test 12: Created_event_for_404_subscription_with_no_local_row_is_noop

    [Fact]
    public async Task Created_event_for_404_subscription_with_no_local_row_is_noop()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            // No subscription row seeded.
            var gateway = new FakeStripeGateway(); // returns null for any id

            var handler = BuildHandler(db, gateway);

            // Should complete without throwing — no row to quarantine.
            var act = async () => await handler.HandleCreatedOrUpdatedAsync("sub_unknown", isCreated: true, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.Subscriptions.CountAsync()).Should().Be(0);
        }
    }

    // ── Test 13: Deleted_event_completes_with_recording_email_queue ─────────

    [Fact]
    public async Task Deleted_event_completes_with_recording_email_queue()
    {
        var (scope, db, _, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            // Use the RecordingEmailQueue explicitly (covers behavior #6: soft coupling)
            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);

            await handler.HandleDeletedAsync("sub_test_1", default);

            emailQueue.Messages.Should().HaveCount(1,
                "billing_subscription_canceled must be enqueued even with a no-op channel reader");
        }
    }

    // ── Test 14: Deleted_event_with_no_owner_skips_email_and_does_not_throw ──

    [Fact]
    public async Task Deleted_event_with_no_owner_skips_email_and_does_not_throw()
    {
        // Seed an org with no OrganizationMember rows — simulates an orphaned subscription
        // where the owner has been deleted. The handler must still cancel the row.
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        // Add user and org, but deliberately omit the OrganizationMember row.
        db.Users.Add(new User { Id = userId, Email = "owner@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "Orphan", Slug = $"orphan-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        // No OrganizationMembers row seeded — owner lookup returns null.
        SeedSubscription(db, orgId);
        await db.SaveChangesAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(status: "canceled"));

            var emailQueue = new RecordingEmailQueue();
            var handler = BuildHandler(db, gateway, emailQueue);

            // Should not throw even without an owner.
            var act = async () => await handler.HandleDeletedAsync("sub_test_1", default);
            await act.Should().NotThrowAsync();

            // No email sent when there is no owner.
            emailQueue.Messages.Should().BeEmpty("no owner to email");

            // Row is still marked Canceled.
            db.ChangeTracker.Clear();
            var row = await db.Subscriptions.SingleAsync(s => s.OrgId == orgId);
            row.Status.Should().Be(SubscriptionStatus.Canceled);
        }
    }

    // ── M16-006: Preemption tests ────────────────────────────────────────────

    [Fact]
    public async Task Created_event_for_paid_tier_preempts_owner_active_trials()
    {
        var (scope, db, userId, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, stripeSubscriptionId: null, tier: SubscriptionTier.Free);
            // Seed full-initial trials for the owner.
            var grantedAt = DateTime.UtcNow.AddDays(-1);
            foreach (var f in TrialFeatures.All)
                db.Trials.Add(new Trial
                {
                    Id = Guid.NewGuid(), UserId = userId, Feature = f,
                    Kind = TrialKind.FullInitial,
                    GrantedAt = grantedAt, ExpiresAt = grantedAt.AddDays(14),
                    CreatedAt = grantedAt, UpdatedAt = grantedAt,
                });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(
                id: "sub_test_1",
                customerId: "cus_test_owner",
                status: "active",
                priceId: "price_test_team_monthly"));
            var handler = BuildHandler(db, gateway);

            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);

            db.ChangeTracker.Clear();
            var rows = await db.Trials.Where(t => t.UserId == userId).ToListAsync();
            rows.Should().OnlyContain(r => r.Kind == TrialKind.PreemptedBySubscription);
            rows.Should().OnlyContain(r => r.ExpiresAt <= DateTime.UtcNow.AddSeconds(1));
        }
    }

    [Fact]
    public async Task Updated_event_does_not_preempt_trials()
    {
        var (scope, db, userId, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, tier: SubscriptionTier.Team);
            var grantedAt = DateTime.UtcNow.AddDays(-1);
            foreach (var f in TrialFeatures.All)
                db.Trials.Add(new Trial
                {
                    Id = Guid.NewGuid(), UserId = userId, Feature = f,
                    Kind = TrialKind.FullInitial,
                    GrantedAt = grantedAt, ExpiresAt = grantedAt.AddDays(14),
                    CreatedAt = grantedAt, UpdatedAt = grantedAt,
                });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub());
            var handler = BuildHandler(db, gateway);

            // isCreated=false → updated event → no preemption
            await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: false, default);

            db.ChangeTracker.Clear();
            var rows = await db.Trials.Where(t => t.UserId == userId).ToListAsync();
            rows.Should().OnlyContain(r => r.Kind == TrialKind.FullInitial,
                because: "updated event must not preempt trials");
        }
    }

    [Fact]
    public async Task Created_event_with_already_preempted_trials_is_a_noop()
    {
        // Idempotency: re-delivery of the create event must not double-update.
        var (scope, db, userId, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, stripeSubscriptionId: null, tier: SubscriptionTier.Free);
            var now = DateTime.UtcNow;
            // Rows already preempted and expired
            foreach (var f in TrialFeatures.All)
                db.Trials.Add(new Trial
                {
                    Id = Guid.NewGuid(), UserId = userId, Feature = f,
                    Kind = TrialKind.PreemptedBySubscription,
                    GrantedAt = now.AddDays(-5), ExpiresAt = now.AddDays(-1),
                    CreatedAt = now.AddDays(-5), UpdatedAt = now.AddDays(-1),
                });
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub());
            var handler = BuildHandler(db, gateway);

            // Should complete without exception — already preempted rows are filtered out.
            var act = async () =>
                await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);
            await act.Should().NotThrowAsync();

            // Still all preempted — unchanged
            db.ChangeTracker.Clear();
            var rows = await db.Trials.Where(t => t.UserId == userId).ToListAsync();
            rows.Should().OnlyContain(r => r.Kind == TrialKind.PreemptedBySubscription);
        }
    }

    [Fact]
    public async Task Created_event_with_no_trial_rows_is_a_noop()
    {
        // No exception, no DB writes when the user has never been seeded.
        var (scope, db, userId, orgId) = await SeedOrgAsync();
        await using (scope)
        {
            SeedSubscription(db, orgId, stripeSubscriptionId: null, tier: SubscriptionTier.Free);
            await db.SaveChangesAsync();

            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub());
            var handler = BuildHandler(db, gateway);

            var act = async () =>
                await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);
            await act.Should().NotThrowAsync();

            db.ChangeTracker.Clear();
            (await db.Trials.CountAsync(t => t.UserId == userId)).Should().Be(0);
        }
    }

    [Fact]
    public async Task Created_event_with_no_org_owner_is_a_noop()
    {
        // Defensive: if OrganizationMembers has no Owner row, preemption is skipped.
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        // Add user and org, deliberately omit OrganizationMember row.
        db.Users.Add(new User { Id = userId, Email = "owner@example.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId, Name = "NoOwner", Slug = $"noowner-{orgId:N}"[..20],
            OwnerId = userId, Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        // No OrganizationMembers row
        db.Subscriptions.Add(new Subscription
        {
            Id = Guid.NewGuid(), OrgId = orgId,
            Tier = SubscriptionTier.Free, Status = SubscriptionStatus.Active,
            Interval = "month", SeatCount = 1, SeatLimit = 1,
            StripeCustomerId = "cus_noowner",
            StripeSubscriptionId = null,
            CurrentPeriodStart = DateTime.UtcNow,
            CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
            CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        await using (scope)
        {
            var gateway = new FakeStripeGateway();
            gateway.SetSubscriptionForTest(MakeStripeSub(id: "sub_test_1", customerId: "cus_noowner"));
            var handler = BuildHandler(db, gateway);

            var act = async () =>
                await handler.HandleCreatedOrUpdatedAsync("sub_test_1", isCreated: true, default);
            await act.Should().NotThrowAsync();
        }
    }

    // ── Tests 15+: MapStatus branch coverage ─────────────────────────────────

    [Theory]
    [InlineData("active",              SubscriptionStatus.Active)]
    [InlineData("trialing",            SubscriptionStatus.Active)]
    [InlineData("past_due",            SubscriptionStatus.PastDue)]
    [InlineData("canceled",            SubscriptionStatus.Canceled)]
    [InlineData("incomplete",          SubscriptionStatus.Incomplete)]
    [InlineData("incomplete_expired",  SubscriptionStatus.Incomplete)]
    [InlineData("unpaid",              SubscriptionStatus.PastDue)]
    [InlineData("paused",              SubscriptionStatus.PastDue)]
    [InlineData("unknown_future_status", SubscriptionStatus.Incomplete)] // default branch
    public async Task MapStatus_returns_correct_local_status(string stripeStatus, SubscriptionStatus expected)
    {
        // MapStatus is now an instance method — need a handler instance.
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();
        await using (scope)
        {
            var handler = BuildHandler(db, new FakeStripeGateway());
            var result = handler.MapStatus(stripeStatus);
            result.Should().Be(expected);
        }
    }
}
