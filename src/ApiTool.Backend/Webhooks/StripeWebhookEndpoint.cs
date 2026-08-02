// Spec refs: docs/SPECIFICATION.md:6818-6854 (signature verification + idempotency
// + 5-failure quarantine + multi-secret rotation + 90-day retention + tolerance
// footgun guard); :9985-10004 (stripe_webhook_events schema).
using System.Text;
using Microsoft.Extensions.Options;
using Stripe;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Webhooks;

/// <summary>Maps the <c>POST /webhooks/stripe</c> endpoint.</summary>
public static class StripeWebhookEndpoint
{
    /// <summary>Registers the webhook endpoint on the route builder.</summary>
    public static IEndpointRouteBuilder MapStripeWebhookEndpoint(this IEndpointRouteBuilder app)
    {
        app.MapPost("/webhooks/stripe", HandleAsync)
            .AllowAnonymous()
            .WithName("StripeWebhook")
            .WithTags("Webhooks");
        return app;
    }

    private static async Task<IResult> HandleAsync(
        HttpContext context,
        IOptions<StripeWebhookOptions> webhookOptions,
        StripeWebhookStore store,
        IStripeWebhookDispatcher dispatcher,
        ILoggerFactory loggerFactory,
        CancellationToken ct)
    {
        // ILogger<T> cannot be used directly with a static class; use the factory with
        // the fully-qualified type name so log category is unambiguous.
        var logger = loggerFactory.CreateLogger(typeof(StripeWebhookEndpoint).FullName!);
        // 1) Read raw body — Stripe signs the exact bytes, no re-encoding.
        string rawBody;
        using (var reader = new StreamReader(context.Request.Body, Encoding.UTF8, leaveOpen: true))
            rawBody = await reader.ReadToEndAsync(ct);

        var signature = context.Request.Headers["Stripe-Signature"].FirstOrDefault();
        if (string.IsNullOrEmpty(signature))
        {
            logger.LogWarning("stripe_webhook_signature_missing");
            return HttpResults.BadRequest();
        }

        // 2) Verify against the multi-secret list — succeed on any match.
        var opts = webhookOptions.Value;
        Event? evt = null;
        var secrets = opts.SecretList();

        // Guard: no secrets configured — cannot verify any signature.
        if (secrets.Count == 0)
        {
            logger.LogWarning("stripe_webhook_no_secrets_configured");
            return HttpResults.BadRequest();
        }

        foreach (var secret in secrets)
        {
            try
            {
                evt = EventUtility.ConstructEvent(
                    rawBody, signature, secret, opts.ToleranceSeconds,
                    throwOnApiVersionMismatch: false);
                break;
            }
            catch (StripeException)
            {
                // Try the next secret.
            }
        }
        if (evt is null)
        {
            logger.LogWarning("stripe_webhook_signature_invalid secrets_tried={Count}", secrets.Count);
            return HttpResults.BadRequest();
        }

        // 3) Idempotency / retry routing.
        var insert = await store.TryInsertPendingAsync(evt.Id, evt.Type, rawBody, ct);
        if (!insert.Inserted)
        {
            // 'processed' → true duplicate: already handled successfully, skip.
            // 'quarantined' → budget exhausted, break retry storm.
            // 'pending' → a previous attempt failed; fall through to re-dispatch (retry semantics).
            if (insert.ExistingStatus is "processed" or "quarantined")
            {
                logger.LogInformation(
                    "stripe_webhook_duplicate event_id={EventId} status={Status} original_received_at={ReceivedAt}",
                    evt.Id, insert.ExistingStatus, insert.ExistingReceivedAt);
                return HttpResults.Ok();
            }
            // pending row — continue to dispatch (Stripe is retrying a failed delivery).
        }

        // 4) Dispatch — M14-011 ships a no-op; M14-012/013 wire real handlers.
        try
        {
            await dispatcher.DispatchAsync(evt, ct);
            await store.MarkProcessedAsync(evt.Id, ct);
            return HttpResults.Ok();
        }
        catch (Exception ex)
        {
            var attempts = await store.RecordFailureAsync(evt.Id, ex.Message, ct);
            if (attempts >= StripeWebhookStore.QuarantineThreshold)
            {
                logger.LogError(ex,
                    "stripe_webhook_quarantined event_id={EventId} event_type={EventType} attempt_count={AttemptCount}",
                    evt.Id, evt.Type, attempts);
                return HttpResults.Ok(); // break the retry storm.
            }
            logger.LogWarning(ex,
                "stripe_webhook_handler_failed event_id={EventId} event_type={EventType} attempt_count={AttemptCount}",
                evt.Id, evt.Type, attempts);
            return HttpResults.StatusCode(StatusCodes.Status500InternalServerError);
        }
    }
}
