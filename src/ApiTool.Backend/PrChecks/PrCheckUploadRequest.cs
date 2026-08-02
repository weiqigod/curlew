using System.Text.Json;

namespace ApiTool.Backend.PrChecks;

/// <summary>
/// Wire DTO for <c>POST /api/v1/pr-checks</c> (v4.2.1 shape).
/// Snake_case JSON via global serializer options.
/// </summary>
public sealed class PrCheckUploadRequest
{
    /// <summary>Repo slug, e.g. <c>owner/name</c>. Required.</summary>
    public string? Repo { get; set; }

    /// <summary>Pull-request number. Must be positive. Required.</summary>
    public int? Pr { get; set; }

    /// <summary>CLI state: one of {success, failure, cancelled, timed_out, neutral, skipped}. Required.</summary>
    public string? State { get; set; }

    /// <summary>40-char hex git commit SHA. Required.</summary>
    public string? HeadSha { get; set; }

    /// <summary>Optional wire-format result id (e.g. <c>res_abc123…</c>).</summary>
    public string? ResultId { get; set; }

    /// <summary>Optional URL for the 'Details' link on the GitHub check run.</summary>
    public string? DetailsUrl { get; set; }

    /// <summary>Optional check-run output block.</summary>
    public PrCheckUploadOutput? Output { get; set; }

    /// <summary>
    /// Provider discriminator. One of: <c>github</c> | <c>gitlab</c>. Optional; defaults to
    /// <c>github</c> when absent for backward compatibility with M14-018 CLI clients.
    /// Refs M16-014 Decision J.
    /// </summary>
    public string? Provider { get; set; }
}

/// <summary>Output block for <see cref="PrCheckUploadRequest"/>.</summary>
public sealed class PrCheckUploadOutput
{
    /// <summary>Short title for the check-run output (≤ 255 chars).</summary>
    public string? Title { get; set; }

    /// <summary>Markdown summary (truncated to 60 000 chars before posting).</summary>
    public string? Summary { get; set; }

    /// <summary>Markdown body text (truncated to 60 000 chars before posting).</summary>
    public string? Text { get; set; }

    /// <summary>Up to 50 annotation objects (excess silently dropped per spec :8525).</summary>
    public IReadOnlyList<JsonElement>? Annotations { get; set; }
}

/// <summary>Response body for a successful <c>POST /api/v1/pr-checks</c>.</summary>
/// <param name="State">Lifecycle state after posting (<c>posted</c> or <c>queued</c>).</param>
/// <param name="GithubCheckRunId">GitHub check_run id when state is <c>posted</c>. Always null for <c>gitlab</c> provider rows.</param>
/// <param name="PrCheckId">Server-assigned pr-check id (wire format).</param>
public sealed record PrCheckUploadResponse(
    string State,
    long? GithubCheckRunId,
    string PrCheckId);
