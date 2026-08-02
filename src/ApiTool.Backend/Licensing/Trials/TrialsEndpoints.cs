// Refs docs/SPECIFICATION.md:5800-5860 (on-demand trial activation).
using ApiTool.Backend.Auth;
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Auth.Shared;
using ApiTool.Backend.Data;
using ApiTool.Backend.Licensing.Tokens;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// Maps <c>POST /api/v1/trials/{feature}</c> — the on-demand trial activation endpoint.
/// Refs docs/SPECIFICATION.md:5800-5860.
/// </summary>
public static class TrialsEndpoints
{
    /// <summary>Registers <c>POST /api/v1/trials/{feature}</c> on the provided route builder.</summary>
    public static IEndpointRouteBuilder MapTrialsEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/trials/{feature}", async (
            string feature,
            TrialActivationRequest? body,
            HttpContext http,
            CurrentUserAccessor users,
            TrialsService trials,
            RefreshTokenService refreshSvc,
            LicenseTokenIssuer licenseIssuer,
            AccessTokenIssuer accessIssuer,
            AppDbContext db,
            CancellationToken ct) =>
        {
            var userId = await users.ResolveAsync(ct);
            if (userId is null)
                return HttpResults.Unauthorized();

            if (body is null
                || string.IsNullOrWhiteSpace(body.RefreshToken)
                || body.DeviceId is null
                || body.DeviceId == Guid.Empty)
                return TrialsProblem.InvalidRequest(http, "refresh_token and device_id are required.");

            var outcome = await trials.ActivateOnDemandAsync(userId.Value, feature, ct);

            switch (outcome)
            {
                case TrialActivationResult.UnknownFeature:
                    return TrialsProblem.UnknownFeature(http, feature);

                case TrialActivationResult.AlreadyConsumed ac:
                    return TrialsProblem.AlreadyConsumed(http, ac);

                case TrialActivationResult.Granted g:
                {
                    var ip = http.Connection.RemoteIpAddress?.ToString();
                    var ua = http.Request.Headers.UserAgent.ToString();

                    var rotate = await refreshSvc.RotateAsync(
                        body.RefreshToken!, body.DeviceId.Value, ip, ua, ct);

                    if (rotate.Outcome != RotateOutcome.Success)
                    {
                        // Map to the same problem shapes as /auth/refresh.
                        return rotate.Outcome switch
                        {
                            RotateOutcome.Reused        => RefreshProblem.Reused(http),
                            RotateOutcome.Expired       => RefreshProblem.Expired(http),
                            RotateOutcome.DeviceMismatch => RefreshProblem.DeviceMismatch(http),
                            _                           => RefreshProblem.Invalid(http, "Refresh token not recognised."),
                        };
                    }

                    // Re-mint tokens. ITrialStateResolver is invoked by LicenseTokenIssuer
                    // automatically; the just-inserted OnDemand row will surface as "active".
                    var (tier, orgId, orgRole, email) = await UserContext.ResolveAsync(db, userId.Value, ct);
                    var (features, requestLimit)      = TierConfig.For(tier);

                    var licenseJwt = await licenseIssuer.IssueAsync(new LicenseTokenInput(
                        UserId:       userId.Value,
                        Email:        email,
                        Tier:         tier,
                        OrgId:        orgId,
                        OrgRole:      orgRole,
                        DeviceId:     body.DeviceId.Value,
                        Features:     features,
                        RequestLimit: requestLimit), ct);

                    var accessToken = await accessIssuer.IssueAsync(new AccessTokenInput(
                        UserId:   userId.Value,
                        Tier:     tier,
                        OrgId:    orgId,
                        DeviceId: body.DeviceId.Value), ct);

                    http.Response.Headers["X-Request-Id"] = http.TraceIdentifier;

                    return HttpResults.Ok(new TrialActivationResponse(
                        Feature:   g.Feature,
                        Kind:      "ondemand",
                        GrantedAt: g.GrantedAt,
                        ExpiresAt: g.ExpiresAt,
                        Tokens:    new TokenTrio(licenseJwt, accessToken, rotate.NewPlaintextToken!)));
                }

                default:
                    return HttpResults.StatusCode(StatusCodes.Status500InternalServerError);
            }
        })
        .RequireAuthorization()
        .DisableAntiforgery()
        .WithName("TrialActivate")
        .WithTags("Trials")
        .Accepts<TrialActivationRequest>("application/json")
        .Produces<TrialActivationResponse>()
        .ProducesProblem(StatusCodes.Status400BadRequest)
        .ProducesProblem(StatusCodes.Status401Unauthorized)
        .ProducesProblem(StatusCodes.Status404NotFound)
        .ProducesProblem(StatusCodes.Status409Conflict);

        return app;
    }

}
