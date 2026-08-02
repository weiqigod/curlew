// Spec refs: docs/SPECIFICATION.md:8551-8597 (signature verification + idempotency
// + 5-failure quarantine + multi-secret rotation + 90-day retention);
// :10031-10049 (github_webhook_events schema).
using System.Text;
using System.Text.Json;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>Maps the <c>POST /webhooks/github</c> endpoint.</summary>
public static class GithubWebhookEndpoint
{
    /// <summary>Registers the webhook endpoint on the route builder.</summary>
    public static IEndpointRouteBuilder MapGithubWebhookEndpoint(this IEndpointRouteBuilder app)
    {
        app.MapPost("/webhooks/github", HandleAsync)
            .AllowAnonymous()
            .WithName("GithubWebhook")
            .WithTags("Webhooks");
        return app;
    }

    private static async Task<IResult> HandleAsync(
        HttpContext context,
        IOptions<GithubWebhookOptions> webhookOptions,
        GithubWebhookStore store,
        IGithubWebhookDispatcher dispatcher,
        ILoggerFactory loggerFactory,
        CancellationToken ct)
    {
        var logger = loggerFactory.CreateLogger(typeof(GithubWebhookEndpoint).FullName!);

        // 1) Read raw body BEFORE JSON parse — spec :8570.
        byte[] rawBody;
        using (var ms = new MemoryStream())
        {
            await context.Request.Body.CopyToAsync(ms, ct);
            rawBody = ms.ToArray();
        }

        // 2) Reject SHA-1-only deliveries — spec :8575.
        if (!context.Request.Headers.TryGetValue("X-Hub-Signature-256", out var sigHeaderValues))
        {
            if (context.Request.Headers.ContainsKey("X-Hub-Signature"))
                logger.LogWarning("github_webhook_legacy_sha1_rejected delivery_id={DeliveryId}",
                    context.Request.Headers["X-GitHub-Delivery"].FirstOrDefault());
            return HttpResults.Unauthorized();
        }
        var signatureHeader = sigHeaderValues.FirstOrDefault();

        // 3) Parse delivery ID & event type headers.
        var deliveryHeader = context.Request.Headers["X-GitHub-Delivery"].FirstOrDefault();
        if (!Guid.TryParse(deliveryHeader, out var deliveryId))
            return HttpResults.BadRequest();
        var eventType = context.Request.Headers["X-GitHub-Event"].FirstOrDefault();
        if (string.IsNullOrEmpty(eventType))
            return HttpResults.BadRequest();

        // 4) Verify signature against every configured secret — spec :8577-8584.
        var opts = webhookOptions.Value;
        var secrets = opts.SecretList();
        if (secrets.Count == 0)
        {
            logger.LogWarning("github_webhook_no_secrets_configured");
            return HttpResults.Unauthorized();
        }
        if (!GithubWebhookSignatureVerifier.TryVerify(rawBody, signatureHeader, secrets))
        {
            logger.LogWarning(
                "github_webhook_signature_invalid delivery_id={DeliveryId} secrets_tried={Count}",
                deliveryId, secrets.Count);
            return HttpResults.Unauthorized();
        }

        // 5) Parse JSON only after signature is verified.
        JsonDocument payload;
        try { payload = JsonDocument.Parse(rawBody); }
        catch (JsonException) { return HttpResults.BadRequest(); }

        var action = payload.RootElement.TryGetProperty("action", out var actionEl)
            ? actionEl.GetString() : null;
        var rawPayloadJson = Encoding.UTF8.GetString(rawBody);

        // 6) Idempotency insert — spec :8586-8595.
        var insert = await store.TryInsertPendingAsync(deliveryId, eventType, action, rawPayloadJson, ct);
        if (!insert.Inserted)
        {
            if (insert.ExistingStatus is "processed" or "quarantined")
            {
                logger.LogInformation(
                    "github_webhook_duplicate delivery_id={DeliveryId} status={Status}",
                    deliveryId, insert.ExistingStatus);
                payload.Dispose();
                return HttpResults.Ok();
            }
            // pending row → fall through to re-dispatch (retry semantics).
        }

        // 7) Dispatch.
        try
        {
            var envelope = new GithubWebhookEnvelope(deliveryId, eventType, action, payload);
            await dispatcher.DispatchAsync(envelope, ct);
            await store.MarkProcessedAsync(deliveryId, ct);
            return HttpResults.Ok();
        }
        catch (Exception ex)
        {
            var attempts = await store.RecordFailureAsync(deliveryId, ex.Message, ct);
            if (attempts >= GithubWebhookStore.QuarantineThreshold)
            {
                logger.LogError(ex,
                    "github_webhook_quarantined delivery_id={DeliveryId} event_type={EventType} attempt_count={AttemptCount}",
                    deliveryId, eventType, attempts);
                return HttpResults.Ok(); // break the retry storm
            }
            logger.LogWarning(ex,
                "github_webhook_handler_failed delivery_id={DeliveryId} event_type={EventType} attempt_count={AttemptCount}",
                deliveryId, eventType, attempts);
            return HttpResults.StatusCode(StatusCodes.Status500InternalServerError);
        }
        finally { payload.Dispose(); }
    }
}
