// Refs docs/SPECIFICATION.md:6796–6804 (event table), :6845–6848 (re-fetch pattern).
// End-to-end test: seeds an org+subscription row, sends a signed webhook payload,
// and asserts both the DB state change and the email queue message.
using System.Net;
using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using ApiTool.Backend.Webhooks;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Webhooks;

/// <summary>
/// Integration test: sends a signed customer.subscription.deleted webhook through the full
/// HTTP stack (BackendFactory) and verifies that the subscription row is marked Canceled and
/// the billing_subscription_canceled email is enqueued.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class StripeSubscriptionHandlerIntegrationTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public StripeSubscriptionHandlerIntegrationTests(BackendFactory factory)
        => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private const string Secret = "whsec_test";

    private static (string body, string sigHeader) BuildSignedDelivery(string eventJson, string secret = Secret)
    {
        var ts = DateTimeOffset.UtcNow.ToUnixTimeSeconds();
        var signed = $"{ts}.{eventJson}";
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
        var sig = Convert.ToHexString(hmac.ComputeHash(Encoding.UTF8.GetBytes(signed))).ToLowerInvariant();
        return (eventJson, $"t={ts},v1={sig}");
    }

    [Fact]
    public async Task CustomerSubscriptionDeleted_marks_canceled_and_enqueues_email()
    {
        const string subId = "sub_integ_deleted_1";
        const string cusId = "cus_integ_owner_1";

        // 1. Create a gateway with the subscription scripted so re-fetch works.
        var gateway = new FakeStripeGateway();
        gateway.SetSubscriptionForTest(new Stripe.Subscription
        {
            Id = subId,
            CustomerId = cusId,
            Status = "canceled",
            CurrentPeriodStart = DateTime.UtcNow.AddMonths(-1),
            CurrentPeriodEnd = DateTime.UtcNow,
            CanceledAt = DateTime.UtcNow,
        });

        // 2. Wire the real handlers (scoped inline) with the scripted gateway.
        var emailQueue = new RecordingEmailQueue();

        // Build an HTTP client that installs the real dispatcher + fake gateway + recording queue.
        var client = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureAppConfiguration((_, cfg) =>
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:Stripe:Webhook:Secrets"] = Secret,
                }));
            b.ConfigureServices(services =>
            {
                // Override gateway with the scripted fake.
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway>(gateway);

                // Override email queue so we can inspect messages.
                services.RemoveAll<IEmailQueue>();
                services.AddSingleton<RecordingEmailQueue>(emailQueue);
                services.AddSingleton<IEmailQueue>(emailQueue);

                // Override the dispatcher with the real one wired to real handlers.
                // We re-register as scoped (using factory lambdas) so they resolve per request.
                services.RemoveAll<IStripeWebhookDispatcher>();
                services.AddScoped<StripeSubscriptionHandler>();
                services.AddScoped<StripeCustomerUpdatedHandler>();
                services.AddScoped<StripeInvoiceHandler>();          // M14-013
                services.AddScoped<StripePaymentMethodHandler>();    // M14-013
                services.AddScoped<IStripeWebhookDispatcher, StripeWebhookDispatcher>();
            });
        }).CreateClient();

        // 3. Seed the org + subscription row in the integration test's DB.
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

            var userId = Guid.NewGuid();
            var orgId = Guid.NewGuid();

            if (!await db.Users.AnyAsync(u => u.Email == "integ-owner@example.com"))
            {
                db.Users.Add(new User { Id = userId, Email = "integ-owner@example.com", CreatedAt = DateTime.UtcNow });
                db.Organizations.Add(new Organization
                {
                    Id = orgId, Name = "IntegOrg", Slug = $"integ-{orgId:N}"[..20],
                    OwnerId = userId, Status = OrgStatus.Active,
                    CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
                });
                db.OrganizationMembers.Add(new OrganizationMember
                {
                    OrgId = orgId, UserId = userId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow,
                });
                db.Subscriptions.Add(new Subscription
                {
                    Id = Guid.NewGuid(), OrgId = orgId,
                    Tier = SubscriptionTier.Team, Status = SubscriptionStatus.Active,
                    Interval = "month", SeatCount = 3, SeatLimit = 3,
                    StripeCustomerId = cusId,
                    StripeSubscriptionId = subId,
                    CurrentPeriodStart = DateTime.UtcNow.AddMonths(-1),
                    CurrentPeriodEnd = DateTime.UtcNow.AddMonths(1),
                    CreatedAt = DateTime.UtcNow, UpdatedAt = DateTime.UtcNow,
                });
                await db.SaveChangesAsync();
            }
        }

        // 4. POST a signed customer.subscription.deleted event.
        // Use a unique event id each run to avoid idempotency store collisions between test runs.
        var eventId = $"evt_integ_del_{Guid.NewGuid():N}";
        var eventJson = $$"""
            {
              "id": "{{eventId}}",
              "object": "event",
              "api_version": "2024-06-20",
              "created": {{DateTimeOffset.UtcNow.ToUnixTimeSeconds()}},
              "livemode": false,
              "pending_webhooks": 0,
              "request": null,
              "type": "customer.subscription.deleted",
              "data": {
                "object": {
                  "id": "{{subId}}",
                  "object": "subscription",
                  "customer": "{{cusId}}"
                }
              }
            }
            """;

        var (body, sig) = BuildSignedDelivery(eventJson);
        var request = new HttpRequestMessage(HttpMethod.Post, "/webhooks/stripe")
        {
            Content = new StringContent(body, Encoding.UTF8, "application/json"),
        };
        request.Headers.Add("Stripe-Signature", sig);

        var response = await client.SendAsync(request);
        response.StatusCode.Should().Be(HttpStatusCode.OK);

        // 5. Assert the subscription row is now Canceled.
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var row = await db.Subscriptions
                .FirstOrDefaultAsync(s => s.StripeCustomerId == cusId);
            row.Should().NotBeNull();
            row!.Status.Should().Be(SubscriptionStatus.Canceled);
            row.StripeSubscriptionId.Should().BeNull();
        }

        // 6. Assert the email was enqueued.
        emailQueue.Messages.Should().HaveCount(1);
        emailQueue.Messages[0].TemplateSlug.Should().Be("billing_subscription_canceled");
        emailQueue.Messages[0].To.Should().Be("integ-owner@example.com");
    }
}
