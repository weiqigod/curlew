namespace ApiTool.Backend.Organizations;

/// <summary>Standard error response body for 4xx and 5xx responses.</summary>
/// <param name="Code">Machine-readable error code.</param>
/// <param name="Message">Human-readable description of the error.</param>
/// <param name="Field">Optional JSON pointer to the invalid field (e.g. "/pass_count").</param>
public sealed record ErrorResponse(string Code, string Message, string? Field = null);
