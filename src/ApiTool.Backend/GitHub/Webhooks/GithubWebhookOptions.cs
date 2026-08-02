namespace ApiTool.Backend.GitHub.Webhooks;

/// <summary>
/// Configuration for the GitHub webhook ingest pipeline. Bound from
/// <c>ApiTool:GitHub:Webhook:*</c> with environment-variable equivalents
/// <c>APITOOL__GITHUB__WEBHOOK__SECRETS</c> (canonical) and <c>GITHUB__WEBHOOK_SECRETS</c>
/// (the spelling shown in the task observable, accepted via Program.cs fallback).
/// Refs docs/SPECIFICATION.md:8577-8584 (multi-secret rotation pattern).
/// </summary>
public sealed class GithubWebhookOptions
{
    /// <summary>Configuration section name.</summary>
    public const string Section = "ApiTool:GitHub:Webhook";

    /// <summary>
    /// Comma-separated list of webhook signing secrets. Multiple values support
    /// the rotation grace window — verification iterates the list and succeeds
    /// on any match. Empty entries are skipped.
    /// </summary>
    public string Secrets { get; set; } = string.Empty;

    /// <summary>Splits <see cref="Secrets"/> on comma, trims whitespace, drops empties.</summary>
    public IReadOnlyList<string> SecretList()
    {
        if (string.IsNullOrWhiteSpace(Secrets)) return Array.Empty<string>();
        return Secrets.Split(',', StringSplitOptions.TrimEntries | StringSplitOptions.RemoveEmptyEntries);
    }
}
