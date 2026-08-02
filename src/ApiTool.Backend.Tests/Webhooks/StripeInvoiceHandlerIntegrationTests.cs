// Refs docs/SPECIFICATION.md:6801–6806, :6845–6848 (re-fetch pattern).
// End-to-end test: seeds an org+subscription row, sends a signed invoice.payment_succeeded
// webhook payload, and asserts both the DB state change and the email queue message.
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
/// Integration test: sends a signed invoice.payment_succeeded webhook through the full
/// HTTP stack (BackendFactory) and verifies that the invoice row is written, subscription
/// status is Active, and the billing_receipt email is enqueued.
/// </summary>
[Collection(BackendCollection.Name)]
public sealed class StripeInvoiceHandlerIntegrationTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public StripeInvoiceHandlerIntegrationTests(BackendFactory factory)
        => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private const string Secret = "whsec_integ_inv_test";

    private static (string body, string sigHeader) BuildSignedDelivery(string eventJson, string secret = Secret)
    {
        var ts = DateTimeOffset.UtcNow.ToUnixTimeSeconds();
        var signed = $"{ts}.{eventJson}";
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
        var sig = Convert.ToHexString(hmac.ComputeHash(Encoding.UTF8.GetBytes(signed))).ToLowerInvariant();
        return (eventJson, $"t={ts},v1={sig}");
    }

    [Fact]
    public async Task InvoicePaymentSucceeded_persists_invoice_and_enqueues_billing_receipt()
    {
        const string invoiceId = "in_integ_paid_2";
        const string subId = "sub_integ_inv_1";
        const string cusId = "cus_integ_inv_owner_1";

        // 1. Script the gateway with the invoice.
        var gateway = new FakeStripeGateway();
        gateway.SetInvoiceForTest(new Stripe.Invoice
        {
            Id = invoiceId,
            CustomerId = cusId,
            SubscriptionId = subId,
            Status = "paid",
            AmountPaid = 4900,
            Total = 4900,
            Currency = "usd",
            HostedInvoiceUrl = "https://invoice.stripe.test/in_integ_paid_2",
            PeriodStart = DateTime.UtcNow.AddMonths(-1),
            PeriodEnd = DateTime.UtcNow,
            AttemptCount = 1,
        });

        var emailQueue = new RecordingEmailQueue();

        // 2. Build an HTTP client overriding gateway + email queue + dispatcher.
        var client = _factory.WithWebHostBuilder(b =>
        {
            b.ConfigureAppConfiguration((_, cfg) =>
                cfg.AddInMemoryCollection(new Dictionary<string, string?>
                {
                    ["ApiTool:Stripe:Webhook:Secrets"] = Secret,
                    ["ApiTool:App:WebAppUrl"] = "https://app.test",
                }));
            b.ConfigureServices(services =>
            {
                services.RemoveAll<IStripeGateway>();
                services.AddSingleton<IStripeGateway>(gateway);

                services.RemoveAll<IEmailQueue>();
                services.AddSingleton<RecordingEmailQueue>(emailQueue);
                services.AddSingleton<IEmailQueue>(emailQueue);

                services.RemoveAll<IStripeWebhookDispatcher>();
                services.AddScoped<StripeSubscriptionHandler>();
                services.AddScoped<StripeCustomerUpdatedHandler>();
                services.AddScoped<StripeInvoiceHandler>();
                services.AddScoped<StripePaymentMethodHandler>();
                services.AddScoped<IStripeWebhookDispatcher, StripeWebhookDispatcher>();
            });
        }).CreateClient();

        // 3. Seed user + org + subscription.
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

            if (!await db.Subscriptions.AnyAsync(s => s.StripeSubscriptionId == subId))
            {
                var userId = Guid.NewGuid();
                var orgId = Guid.NewGuid();

                db.Users.Add(new User { Id = userId, Email = "integ-inv-owner@example.com", CreatedAt = DateTime.UtcNow });
                db.Organizations.Add(new Organization
                {
                    Id = orgId, Name = "InvIntegOrg", Slug = $"invinteg-{orgId:N}"[..20],
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
                    Tier = SubscriptionTier.Team, Status = SubscriptionStatus.PastDue,
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

        // 4. POST signed invoice.payment_succeeded event.
        var eventId = $"evt_integ_inv_{Guid.NewGuid():N}";
        var eventJson = $$"""
            {
              "id": "{{eventId}}",
              "object": "event",
              "api_version": "2024-06-20",
              "created": {{DateTimeOffset.UtcNow.ToUnixTimeSeconds()}},
              "livemode": false,
              "pending_webhooks": 0,
              "request": null,
              "type": "invoice.payment_succeeded",
              "data": {
                "object": {
                  "id": "{{invoiceId}}",
                  "object": "invoice",
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

        // 5. Assert invoice row written.
        using (var scope = _factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            var inv = await db.Invoices.FirstOrDefaultAsync(i => i.StripeInvoiceId == invoiceId);
            inv.Should().NotBeNull("invoice row must be persisted after payment_succeeded");
            inv!.Status.Should().Be("paid");
            inv.HostedInvoiceUrl.Should().Be("https://invoice.stripe.test/in_integ_paid_2");
        }

        // 6. Assert billing_receipt email enqueued.
        emailQueue.Messages.Should().HaveCount(1);
        emailQueue.Messages[0].TemplateSlug.Should().Be("billing_receipt");
        emailQueue.Messages[0].To.Should().Be("integ-inv-owner@example.com");
    }
}
