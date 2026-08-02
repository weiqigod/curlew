// Registers the password-reset endpoints.
// Refs docs/SPECIFICATION.md:8461-8493 (reset flow).
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>Extension method that registers the password-reset endpoint pair.</summary>
public static class PasswordResetEndpoints
{
    /// <summary>Maps the two password-reset endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapPasswordResetEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/auth/password-reset/request", async (
            PasswordResetRequest? body, HttpContext http,
            IPasswordResetService svc, CancellationToken ct) =>
        {
            var email = body?.Email?.Trim() ?? string.Empty;
            var ip    = http.Connection.RemoteIpAddress?.ToString();
            var ua    = http.Request.Headers.UserAgent.ToString();
            await svc.RequestAsync(email, ip, ua, ct);
            return HttpResults.Ok(new PasswordResetResponse(
                Ok: true,
                Message: "If an account exists for that email, a reset link has been sent."));
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .RequireRateLimiting("auth-password-reset-request-ip")
        .WithName("PasswordResetRequest")
        .WithTags("Auth")
        .Accepts<PasswordResetRequest>("application/json")
        .Produces<PasswordResetResponse>();

        app.MapPost("/api/v1/auth/password-reset/confirm", async (
            PasswordResetConfirmRequest? body, HttpContext http,
            IPasswordResetService svc, CancellationToken ct) =>
        {
            var token = body?.Token ?? string.Empty;
            var pw    = body?.NewPassword ?? string.Empty;
            var (err, score) = await svc.ConfirmAsync(token, pw, ct);
            return err switch
            {
                PasswordResetError.None            => (IResult)HttpResults.Ok(new PasswordResetResponse(true, "Password updated.")),
                PasswordResetError.TokenInvalid    => PasswordResetProblem.TokenInvalid(http),
                PasswordResetError.PasswordTooWeak => PasswordResetProblem.PasswordTooWeak(http, score ?? 0),
                _                                  => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .WithName("PasswordResetConfirm")
        .WithTags("Auth")
        .Accepts<PasswordResetConfirmRequest>("application/json")
        .Produces<PasswordResetResponse>()
        .ProducesProblem(StatusCodes.Status400BadRequest)
        .ProducesProblem(StatusCodes.Status422UnprocessableEntity);

        return app;
    }
}
