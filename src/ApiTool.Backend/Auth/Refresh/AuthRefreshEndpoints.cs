// Refs docs/SPECIFICATION.md:8228 (endpoint table), :7901-7944 (rotation semantics).
// Unified mint endpoint: POST /api/v1/auth/refresh
// Mints: License JWT (30d, typ=license+jwt), Access token (1h, typ=at+jwt),
//        opaque refresh token (90/365d).
// Error codes: AUTH_REFRESH_REUSED, AUTH_REFRESH_EXPIRED, AUTH_DEVICE_MISMATCH, AUTH_INVALID_REFRESH.
// See docs/api-errors.md for RFC 7807 error-code reference.
using ApiTool.Backend.Auth.Shared;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Tokens;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth.Refresh;

/// <summary>
/// Maps <c>POST /api/v1/auth/refresh</c> — the unified mint endpoint.
/// Refs docs/SPECIFICATION.md:8228 and :7901-7944.
/// </summary>
public static class AuthRefreshEndpoints
{
    /// <summary>Registers <c>POST /api/v1/auth/refresh</c> on the provided route builder.</summary>
    public static IEndpointRouteBuilder MapAuthRefreshEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/auth/refresh", async (
            AuthRefreshRequest? body,
            HttpContext http,
            RefreshTokenService refreshSvc,
            LicenseTokenIssuer licenseIssuer,
            AccessTokenIssuer accessIssuer,
            AppDbContext db,
            CancellationToken ct) =>
        {
            // Validate request shape
            if (body is null
                || string.IsNullOrWhiteSpace(body.RefreshToken)
                || body.DeviceId is null
                || body.DeviceId == Guid.Empty)
            {
                return RefreshProblem.Invalid(http, "refresh_token and device_id are required.");
            }

            var ip = http.Connection.RemoteIpAddress?.ToString();
            var ua = http.Request.Headers.UserAgent.ToString();

            // Rotate
            var result = await refreshSvc.RotateAsync(
                body.RefreshToken, body.DeviceId.Value, ip, ua, ct);

            // Error paths
            return result.Outcome switch
            {
                RotateOutcome.Reused        => RefreshProblem.Reused(http),
                RotateOutcome.Expired       => RefreshProblem.Expired(http),
                RotateOutcome.DeviceMismatch => RefreshProblem.DeviceMismatch(http),
                RotateOutcome.NotFound      => RefreshProblem.Invalid(http, "Refresh token not recognised."),
                RotateOutcome.Revoked       => RefreshProblem.Invalid(http, "Refresh token not recognised."),
                _ => await MintAndRespondAsync(
                    result, body.DeviceId.Value, db, licenseIssuer, accessIssuer, http, ct),
            };
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .WithName("AuthRefresh")
        .WithTags("Auth")
        .Accepts<AuthRefreshRequest>("application/json")
        .Produces<AuthRefreshResponse>()
        .ProducesProblem(StatusCodes.Status400BadRequest)
        .ProducesProblem(StatusCodes.Status401Unauthorized);

        return app;
    }

    // ── Private helpers ───────────────────────────────────────────────────────

    private static async Task<IResult> MintAndRespondAsync(
        RotateResult result,
        Guid deviceId,
        AppDbContext db,
        LicenseTokenIssuer licenseIssuer,
        AccessTokenIssuer accessIssuer,
        HttpContext http,
        CancellationToken ct)
    {
        var userId = result.UserId!.Value;
        var (tier, orgId, orgRole, email) = await UserContext.ResolveAsync(db, userId, ct);
        var (features, requestLimit)      = TierConfig.For(tier);

        var licenseJwt = await licenseIssuer.IssueAsync(new LicenseTokenInput(
            UserId:       userId,
            Email:        email,
            Tier:         tier,
            OrgId:        orgId,
            OrgRole:      orgRole,
            DeviceId:     deviceId,
            Features:     features,
            RequestLimit: requestLimit), ct);

        var accessToken = await accessIssuer.IssueAsync(new AccessTokenInput(
            UserId:   userId,
            Tier:     tier,
            OrgId:    orgId,
            DeviceId: deviceId), ct);

        http.Response.Headers["X-Request-Id"] = http.TraceIdentifier;

        return HttpResults.Ok(new AuthRefreshResponse(
            LicenseJwt:   licenseJwt,
            AccessToken:  accessToken,
            RefreshToken: result.NewPlaintextToken!));
    }

}
