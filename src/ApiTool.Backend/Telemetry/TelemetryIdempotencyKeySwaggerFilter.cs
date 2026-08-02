using Microsoft.OpenApi.Models;
using Swashbuckle.AspNetCore.SwaggerGen;

namespace ApiTool.Backend.Telemetry;

/// <summary>
/// Swashbuckle <see cref="IOperationFilter"/> that appends the required
/// <c>Idempotency-Key</c> header parameter to the telemetry ingest operation.
/// Swashbuckle's minimal-API support does not auto-discover headers read via
/// <c>HttpContext.Request.Headers</c>, so we inject the parameter manually.
/// </summary>
public sealed class TelemetryIdempotencyKeySwaggerFilter : IOperationFilter
{
    /// <inheritdoc/>
    public void Apply(OpenApiOperation operation, OperationFilterContext context)
    {
        // Only apply to the telemetry ingest endpoint.
        if (context.ApiDescription.RelativePath != "api/v1/telemetry/events")
            return;

        operation.Parameters ??= [];

        // Avoid adding duplicate if already present.
        if (operation.Parameters.Any(p =>
            p.Name == "Idempotency-Key" &&
            p.In == ParameterLocation.Header))
            return;

        operation.Parameters.Add(new OpenApiParameter
        {
            Name = "Idempotency-Key",
            In = ParameterLocation.Header,
            Required = true,
            Schema = new OpenApiSchema
            {
                Type = "string",
                MaxLength = 64,
            },
            Description = "Client-supplied idempotency key (UUID v4 recommended). " +
                          "Replaying the same key returns 202 without re-inserting the event.",
        });
    }
}
