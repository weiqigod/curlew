// Registers the email-verification endpoints.
// Refs docs/SPECIFICATION.md:8495-8509 (verification flow).
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>Extension method that registers the email-verification endpoint pair.</summary>
public static class EmailVerificationEndpoints
{
    /// <summary>Maps the two email-verification endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapEmailVerificationEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/auth/email-verification/resend", async (
            EmailVerificationRequest? body, HttpContext http,
            IEmailVerificationService svc, CancellationToken ct) =>
        {
            var email = body?.Email?.Trim() ?? string.Empty;
            await svc.ResendAsync(email, ct);
            return HttpResults.Ok(new EmailVerificationResponse(
                Ok: true,
                Message: "If an account exists for that email, a verification link has been sent."));
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .RequireRateLimiting("auth-email-verification-resend-ip")
        .WithName("EmailVerificationResend")
        .WithTags("Auth")
        .Accepts<EmailVerificationRequest>("application/json")
        .Produces<EmailVerificationResponse>();

        app.MapPost("/api/v1/auth/email-verification/confirm", async (
            EmailVerificationConfirmRequest? body, HttpContext http,
            IEmailVerificationService svc, CancellationToken ct) =>
        {
            var token = body?.Token ?? string.Empty;
            var err = await svc.ConfirmAsync(token, ct);
            return err switch
            {
                EmailVerificationError.None         => (IResult)HttpResults.Ok(new EmailVerificationResponse(Ok: true, Message: "Email address verified.")),
                EmailVerificationError.TokenInvalid => EmailVerificationProblem.TokenInvalid(http),
                _                                   => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
            };
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .WithName("EmailVerificationConfirm")
        .WithTags("Auth")
        .Accepts<EmailVerificationConfirmRequest>("application/json")
        .Produces<EmailVerificationResponse>()
        .ProducesProblem(StatusCodes.Status400BadRequest);

        return app;
    }
}
