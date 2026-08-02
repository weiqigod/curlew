using System.Text.Json.Serialization;

namespace ApiTool.Backend.Results.Dashboard;

/// <summary>Response envelope for <c>GET /results/failures</c>.</summary>
public sealed record FailuresResponse(
    [property: JsonPropertyName("window")]        string Window,
    [property: JsonPropertyName("limit")]         int Limit,
    [property: JsonPropertyName("limit_clamped")] bool LimitClamped,
    [property: JsonPropertyName("items")]         IReadOnlyList<FailureGroup> Items);

/// <summary>Failure group keyed by <c>(method, path_template)</c>.</summary>
public sealed record FailureGroup(
    [property: JsonPropertyName("method")]          string Method,
    [property: JsonPropertyName("path_template")]   string PathTemplate,
    [property: JsonPropertyName("failure_count")]   int FailureCount,
    [property: JsonPropertyName("first_seen_at")]   DateTime FirstSeenAt,
    [property: JsonPropertyName("last_seen_at")]    DateTime LastSeenAt,
    [property: JsonPropertyName("sample_run_ids")]  IReadOnlyList<string> SampleRunIds);
