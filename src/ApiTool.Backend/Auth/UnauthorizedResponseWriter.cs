using System.Text.Json;
using Microsoft.AspNetCore.Authentication.JwtBearer;

namespace ApiTool.Backend.Auth;

/// <summary>Overrides the default JWT challenge response with a structured JSON error body.</summary>
public static class UnauthorizedResponseWriter
{
    private static readonly byte[] Body = JsonSerializer.SerializeToUtf8Bytes(
        new
        {
            type = "https://api.apitool.dev/errors/unauthorized",
            title = "Unauthorized",
            status = StatusCodes.Status401Unauthorized,
            detail = "Authentication is required. Provide a valid Bearer token.",
            code = "unauthorized",
        });

    /// <summary>
    /// Writes a 401 response with the <c>unauthorized</c> error code.
    /// Intended for use as <see cref="JwtBearerEvents.OnChallenge"/>.
    /// </summary>
    public static async Task WriteAsync(JwtBearerChallengeContext context)
    {
        context.HandleResponse();

        context.Response.StatusCode = StatusCodes.Status401Unauthorized;
        context.Response.ContentType = "application/problem+json";
        await context.Response.Body.WriteAsync(Body);
    }
}
