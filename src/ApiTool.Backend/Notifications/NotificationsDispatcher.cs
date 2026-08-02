using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Notifications;

/// <summary>
/// Implements <see cref="IResultIngestedNotifier"/> by dispatching notifications to
/// matching rules via Slack webhooks or email when a failing run is ingested.
/// Replaces <see cref="NoopResultIngestedNotifier"/> in production.
/// </summary>
public sealed class NotificationsDispatcher(
    IServiceScopeFactory scopeFactory,
    ISlackWebhookPoster slack,
    ISmtpSender smtp,
    TimeProvider clock,
    IOptions<NotificationsDispatcherOptions> options,
    ILogger<NotificationsDispatcher> logger) : IResultIngestedNotifier
{
    private readonly NotificationsDispatcherOptions _options = options.Value;

    /// <inheritdoc/>
    public async Task NotifyAsync(Guid orgId, Guid resultId, CancellationToken ct)
    {
        try
        {
            await DispatchAsync(orgId, resultId, ct);
        }
        catch (Exception ex)
        {
            logger.LogError(ex,
                "Unexpected error in NotificationsDispatcher for result {ResultId} in org {OrgId}",
                resultId, orgId);
        }
    }

    // ── private helpers ──────────────────────────────────────────────────────

    private async Task DispatchAsync(Guid orgId, Guid resultId, CancellationToken ct)
    {
        await using var scope = scopeFactory.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var svc = scope.ServiceProvider.GetRequiredService<NotificationsService>();

        // Load the result to determine whether it's a failing run.
        var result = await db.Results.FindAsync([resultId], ct);
        if (result is null)
        {
            logger.LogWarning("Result {ResultId} not found for notification dispatch", resultId);
            return;
        }

        // Determine which event fired (if any).
        if (result.FailCount == 0)
            return; // Passing run — no dispatch needed.

        var evt = NotificationEvent.RunFailed;

        // Find matching rules for this org + event.
        var rules = await svc.FindMatchingRulesAsync(orgId, evt, ct);
        if (rules.Count == 0)
            return;

        // Build a compact payload JSON.
        var payload = BuildPayload(result);

        foreach (var rule in rules)
        {
            await AttemptDeliveryAsync(svc, rule, result, payload, ct);
        }
    }

    private async Task AttemptDeliveryAsync(
        NotificationsService svc,
        NotificationRule rule,
        Result result,
        string payload,
        CancellationToken ct)
    {
        var delays = _options.RetryDelays;
        var maxAttempts = 1 + delays.Length; // initial + retries

        int? lastResponseCode = null;
        string? lastError = null;

        for (var attempt = 1; attempt <= maxAttempts; attempt++)
        {
            try
            {
                int statusCode;
                if (rule.Channel == NotificationChannel.Slack)
                {
                    statusCode = await slack.PostAsync(rule.Target, payload, ct);
                    lastResponseCode = statusCode;
                    if (statusCode is >= 200 and < 300)
                    {
                        await RecordDelivery(svc, rule, result, attempt,
                            NotificationDeliveryStatus.Delivered, statusCode, null, clock, ct);
                        return;
                    }
                    lastError = $"Webhook returned HTTP {statusCode}";
                }
                else if (rule.Channel == NotificationChannel.Email)
                {
                    await smtp.SendAsync(rule.Target, BuildEmailSubject(result), payload, ct);
                    await RecordDelivery(svc, rule, result, attempt,
                        NotificationDeliveryStatus.Delivered, null, null, clock, ct);
                    return;
                }
            }
            catch (Exception ex)
            {
                lastError = ex.Message;
                lastResponseCode = null;
                logger.LogWarning(ex,
                    "Notification attempt {Attempt}/{Max} failed for rule {RuleId}",
                    attempt, maxAttempts, rule.Id);
            }

            // Wait before retry (if more retries remain)
            if (attempt < maxAttempts && delays.Length >= attempt)
            {
                var delay = delays[attempt - 1];
                if (delay > TimeSpan.Zero)
                    await Task.Delay(delay, ct);
            }
        }

        // All attempts exhausted without a successful return — record failure.
        await RecordDelivery(svc, rule, result, maxAttempts,
            NotificationDeliveryStatus.Failed, lastResponseCode, lastError, clock, ct);
    }

    private static async Task RecordDelivery(
        NotificationsService svc,
        NotificationRule rule,
        Result result,
        int attemptCount,
        NotificationDeliveryStatus status,
        int? responseCode,
        string? errorMessage,
        TimeProvider clock,
        CancellationToken ct)
    {
        var delivery = new NotificationDelivery
        {
            Id = Guid.NewGuid(),
            RuleId = rule.Id,
            OrgId = rule.OrgId,
            ResultId = result.Id,
            Channel = rule.Channel,
            Status = status,
            ResponseCode = responseCode,
            AttemptCount = attemptCount,
            ErrorMessage = errorMessage,
            AttemptedAt = clock.GetUtcNow().UtcDateTime,
        };
        await svc.RecordDeliveryAsync(delivery, ct);
    }

    private static string BuildPayload(Result result)
    {
        var obj = new
        {
            collection_name = result.CollectionName,
            result_id = "res_" + result.Id.ToString("N"),
            run_at = result.RunAt,
            pass_count = result.PassCount,
            fail_count = result.FailCount,
            skipped_count = result.SkippedCount,
            triggered_by = result.TriggeredBy,
            git_sha = result.GitSha,
        };
        return JsonSerializer.Serialize(obj, new JsonSerializerOptions
        {
            PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
            DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull,
        });
    }

    private static string BuildEmailSubject(Result result) =>
        $"[ApiTool] Run failed: {result.CollectionName} ({result.FailCount} failure(s))";
}
