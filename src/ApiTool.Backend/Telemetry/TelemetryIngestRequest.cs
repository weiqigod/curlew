using System.Text.Json.Nodes;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// Request body for <c>POST /api/v1/telemetry/events</c>.
/// All validation is performed by the endpoint handler; model binding provides
/// basic structural deserialization only.
/// </summary>
public sealed class TelemetryIngestRequest
{
    /// <summary>Anonymous identifier of the CLI installation. Must be a valid UUID.</summary>
    public string? InstallId { get; set; }

    /// <summary>Dot-namespaced event type (e.g. <c>run.completed</c>). Open-ended for forward-compat.</summary>
    public string? EventType { get; set; }

    /// <summary>Opaque JSON payload from the CLI. May be empty object <c>{}</c>.</summary>
    public JsonObject? EventPayload { get; set; }
}
