// Spec refs: docs/SPECIFICATION.md:9257-9263 (X-Gitlab-Token verification, idempotency,
// quarantine, Pipeline Hook handler, Push/MR stored-only).
// Mirrors GithubWebhookEndpoint; see plan Decisions A-F for architectural rationale.
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.GitLab.Webhooks;

/// <summary>
/// Maps the <c>POST /webhooks/gitlab</c> endpoint.
/// Single-secret-per-installation only (Open Decision 3).
/// Multi-secret rotation via <c>GITLAB__WEBHOOK_SECRETS_&lt;installation_id&gt;</c> is a documented follow-up.
/// </summary>
public static class GitLabWebhookEndpoint
{
    /// <summary>Registers the webhook endpoint on the route builder.</summary>
    public static IEndpointRouteBuilder MapGitLabWebhookEndpoint(this IEndpointRouteBuilder app)
    {
        app.MapPost("/webhooks/gitlab", HandleAsync)
            .AllowAnonymous()
            .WithName("GitLabWebhook")
            .WithTags("Webhooks")
            .Produces(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status400BadRequest)
            .Produces(StatusCodes.Status401Unauthorized)
            .Produces(StatusCodes.Status429TooManyRequests)
            .Produces(StatusCodes.Status500InternalServerError);
        return app;
    }

    private static async Task<IResult> HandleAsync(
        HttpContext context,
        GitLabWebhookStore store,
        IGitLabWebhookDispatcher dispatcher,
        GitLabWebhookSourceTracker sourceTracker,
        IGitLabKeyProvider keyProvider,
        AppDbContext db,  // used only for installation lookup (TryFindMatchingInstallationAsync)
        ILoggerFactory loggerFactory,
        CancellationToken ct)
    {
        var logger = loggerFactory.CreateLogger(typeof(GitLabWebhookEndpoint).FullName!);

        // 1) Source-IP quarantine check (pre-everything — Decision C).
        var ip = context.Connection.RemoteIpAddress?.ToString() ?? "unknown";
        var (quarantined, quarantinedAt) = sourceTracker.GetState(ip);
        if (quarantined)
        {
            logger.LogWarning(
                "gitlab_webhook_source_quarantined ip={Ip} quarantined_at={QuarantinedAt:o}", ip, quarantinedAt);
            return HttpResults.Json(
                new { quarantined_at = quarantinedAt },
                statusCode: StatusCodes.Status429TooManyRequests);
        }

        // 2) Read raw body BEFORE any parsing.
        byte[] rawBody;
        using (var ms = new MemoryStream())
        {
            await context.Request.Body.CopyToAsync(ms, ct);
            rawBody = ms.ToArray();
        }

        // 3) Read required headers.
        var token = context.Request.Headers["X-Gitlab-Token"].FirstOrDefault();
        var eventUuid = context.Request.Headers["X-Gitlab-Event-UUID"].FirstOrDefault();
        var eventType = context.Request.Headers["X-Gitlab-Event"].FirstOrDefault();

        // 4) Validate header presence.
        if (string.IsNullOrEmpty(token))
        {
            logger.LogWarning("gitlab_webhook_missing_token ip={Ip}", ip);
            return HttpResults.Unauthorized();
        }
        if (string.IsNullOrEmpty(eventUuid))
        {
            logger.LogWarning("gitlab_webhook_missing_event_uuid ip={Ip}", ip);
            return HttpResults.BadRequest();
        }
        if (string.IsNullOrEmpty(eventType))
        {
            logger.LogWarning("gitlab_webhook_missing_event_type ip={Ip}", ip);
            return HttpResults.BadRequest();
        }

        // 5) Iterate installations — constant-time compare, no short-circuit (Decision A).
        //    TODO (performance follow-up): add per-process LRU cache of (InstallationId → SecretPlaintext)
        //    for backends with many installations that use Google KMS (one HTTP round-trip per installation).
        var matchedInstallationId = await TryFindMatchingInstallationAsync(db, keyProvider, token, ct);

        if (matchedInstallationId is null)
        {
            var failures = sourceTracker.RecordFailure(ip);
            logger.LogWarning(
                "gitlab_webhook_verification_failed ip={Ip} failure_count={Count}", ip, failures);

            if (failures >= GitLabWebhookSourceTracker.QuarantineThreshold)
            {
                // Read quarantined_at back from the tracker (set by RecordFailure on threshold crossing)
                // to ensure consistency with the pre-existing quarantine path and with FakeClock in tests.
                var (_, newQuarantinedAt) = sourceTracker.GetState(ip);
                return HttpResults.Json(
                    new { quarantined_at = newQuarantinedAt },
                    statusCode: StatusCodes.Status429TooManyRequests);
            }
            return HttpResults.Unauthorized();
        }

        sourceTracker.RecordSuccess(ip);

        // 6) Parse JSON only after token is verified — Defense in depth (Decision I).
        JsonDocument payload;
        try { payload = JsonDocument.Parse(rawBody); }
        catch (JsonException)
        {
            logger.LogWarning("gitlab_webhook_invalid_json event_uuid={Uuid} event_type={EventType}", eventUuid, eventType);
            return HttpResults.BadRequest();
        }

        // 7) Idempotency insert.
        var insertResult = await store.TryInsertPendingAsync(
            eventUuid, eventType, matchedInstallationId.Value,
            Encoding.UTF8.GetString(rawBody), ct);

        if (!insertResult.Inserted)
        {
            // Already processed or quarantined — idempotent no-op.
            if (insertResult.ExistingProcessed || insertResult.ExistingQuarantined)
            {
                payload.Dispose();
                logger.LogInformation(
                    "gitlab_webhook_duplicate event_uuid={Uuid} event_type={EventType} processed={Processed} quarantined={Quarantined}",
                    eventUuid, eventType, insertResult.ExistingProcessed, insertResult.ExistingQuarantined);
                return HttpResults.Ok();
            }
            // Pending row (ProcessedAt IS NULL, not quarantined) → fall through to re-dispatch (retry semantics).
        }

        // 8) Dispatch.
        try
        {
            var envelope = new GitLabWebhookEnvelope(eventUuid, eventType, matchedInstallationId.Value, payload);
            await dispatcher.DispatchAsync(envelope, ct);
            await store.MarkProcessedAsync(eventUuid, ct);
            return HttpResults.Ok();
        }
        catch (Exception ex)
        {
            var attempts = await store.RecordFailureAsync(eventUuid, ex.Message, ct);
            if (attempts >= GitLabWebhookStore.QuarantineThreshold)
            {
                logger.LogError(ex,
                    "gitlab_webhook_quarantined event_uuid={Uuid} event_type={EventType} attempt_count={Count}",
                    eventUuid, eventType, attempts);
                return HttpResults.Ok(); // break the retry storm
            }
            logger.LogWarning(ex,
                "gitlab_webhook_handler_failed event_uuid={Uuid} event_type={EventType} attempt_count={Count}",
                eventUuid, eventType, attempts);
            return HttpResults.StatusCode(StatusCodes.Status500InternalServerError);
        }
        finally { payload.Dispose(); }
    }

    /// <summary>
    /// Iterates all active installations with a webhook secret and constant-time-compares
    /// the supplied token against each decrypted secret. Iterates every row even after a
    /// match (no short-circuit on match) to avoid leaking timing information about the
    /// number of installations (Decision A).
    /// Returns the matching installation id, or null if no installation matches.
    /// </summary>
    private static async Task<Guid?> TryFindMatchingInstallationAsync(
        AppDbContext db,
        IGitLabKeyProvider keyProvider,
        string token,
        CancellationToken ct)
    {
        var installations = await db.GitLabInstallations
            .Where(x => x.WebhookSecretCiphertext != null && x.DeletedAt == null)
            .OrderBy(x => x.CreatedAt)
            .Select(x => new { x.Id, x.WebhookSecretCiphertext })
            .ToListAsync(ct);

        var tokenBytes = Encoding.UTF8.GetBytes(token);
        Guid? matched = null;

        foreach (var inst in installations)
        {
            try
            {
                // FakeGitLabKeyProvider has no AccessTokenKid on webhook secret; pass empty kid.
                var secretBytes = await keyProvider.DecryptAsync(inst.WebhookSecretCiphertext!, string.Empty, ct);
                if (CryptographicOperations.FixedTimeEquals(tokenBytes, secretBytes))
                    matched ??= inst.Id; // record first match but continue iterating
            }
            catch (Exception ex) when (ex is not OperationCanceledException)
            {
                // Decrypt failure (key mismatch, corrupt ciphertext) — skip this installation.
                // OperationCanceledException is re-thrown so client-disconnect cancellation propagates.
            }
        }

        return matched;
    }
}
