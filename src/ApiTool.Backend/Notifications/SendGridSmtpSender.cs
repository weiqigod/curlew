// Spec refs: docs/SPECIFICATION.md:8866-8953
//   (Email Service Integration; SendGrid Dynamic Templates;
//    manifest variable allowlist; CI upload flow).
using ApiTool.Backend.Notifications.Email;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using SendGrid;
using SendGrid.Helpers.Mail;

namespace ApiTool.Backend.Notifications;

/// <summary>
/// Live <see cref="ISmtpSender"/> backed by the SendGrid 9.x Dynamic Templates API.
/// Registered only when <c>ApiTool:SendGrid:Mode == "live"</c>.
/// </summary>
public sealed class SendGridSmtpSender : ISmtpSender
{
    private readonly ISendGridClient _client;
    private readonly SendGridOptions _opts;
    private readonly EmailTemplateLoader _templates;
    private readonly ILogger<SendGridSmtpSender> _log;

    /// <summary>Initialises the sender. Optionally inject a stub client for testing.</summary>
    public SendGridSmtpSender(
        IOptions<SendGridOptions> options,
        EmailTemplateLoader templates,
        ILogger<SendGridSmtpSender> log,
        ISendGridClient? client = null)
    {
        _opts = options.Value;
        _templates = templates;
        _log = log;
        _client = client ?? new SendGridClient(_opts.ApiKey);
    }

    /// <inheritdoc/>
    public async Task SendAsync(string toAddress, string subject, string body, CancellationToken ct)
    {
        var msg = MailHelper.CreateSingleEmail(
            new EmailAddress(_opts.FromEmail, _opts.FromName),
            new EmailAddress(toAddress),
            subject,
            plainTextContent: body,
            htmlContent: null);
        var resp = await _client.SendEmailAsync(msg, ct).ConfigureAwait(false);
        ThrowOnFailure(resp);
        _log.LogInformation("SendGrid: sent email to {To} subject '{Subject}'", toAddress, subject);
    }

    /// <inheritdoc/>
    public async Task SendTemplateAsync(
        string toAddress,
        string slug,
        IDictionary<string, string> variables,
        CancellationToken ct)
    {
        // 1) Resolve template id — throw before any HTTP if missing.
        if (!_opts.Templates.TryGetValue(slug, out var templateId) || string.IsNullOrEmpty(templateId))
            throw new EmailTemplateNotFoundException(slug);

        // 2) Validate variable allowlist — throw before any HTTP if unknown keys present.
        var manifest = _templates.LoadManifest(slug);
        var unknown = variables.Keys
            .Where(k => !manifest.Variables.ContainsKey(k))
            .ToList();
        if (unknown.Count > 0)
            throw new EmailTemplateVariableUnknownException(slug, unknown);

        // 3) Build and send via SendGrid Dynamic Templates.
        var msg = new SendGridMessage
        {
            From = new EmailAddress(_opts.FromEmail, _opts.FromName),
            TemplateId = templateId,
        };
        msg.AddTo(new EmailAddress(toAddress));
        msg.SetTemplateData(variables.ToDictionary(kv => kv.Key, kv => (object)kv.Value));

        var resp = await _client.SendEmailAsync(msg, ct).ConfigureAwait(false);
        ThrowOnFailure(resp);
        _log.LogInformation("SendGrid: sent template '{Slug}' to {To}", slug, toAddress);
    }

    private static void ThrowOnFailure(Response resp)
    {
        var code = (int)resp.StatusCode;
        if (code >= 500)
            throw new SendGridUnavailableException(code);
        if (code == 429)
            throw new SendGridRateLimitedException();
        if (code >= 400)
            throw new SendGridPermanentException(code);
    }

    // ── Exception hierarchy ────────────────────────────────────────────────

    /// <summary>Thrown when SendGrid returns a 5xx transient error.</summary>
    public sealed class SendGridUnavailableException(int statusCode)
        : Exception($"SendGrid returned {statusCode} (transient; will retry).")
    {
        /// <summary>The HTTP status code returned by SendGrid.</summary>
        public int StatusCode { get; } = statusCode;
    }

    /// <summary>Thrown when SendGrid returns 429 Too Many Requests.</summary>
    public sealed class SendGridRateLimitedException()
        : Exception("SendGrid returned 429 Too Many Requests (transient; will retry).");

    /// <summary>Thrown when SendGrid returns a permanent 4xx error (excluding 429).</summary>
    public sealed class SendGridPermanentException(int statusCode)
        : Exception($"SendGrid returned {statusCode} (permanent; will not retry).")
    {
        /// <summary>The HTTP status code returned by SendGrid.</summary>
        public int StatusCode { get; } = statusCode;
    }
}
