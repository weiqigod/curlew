using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Sso;

/// <summary>Registers the three OIDC SSO endpoints.</summary>
public static class OidcEndpoints
{
    /// <summary>Maps all OIDC SSO endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapOidcEndpoints(this IEndpointRouteBuilder app)
    {
        // ── PUT /api/v1/organizations/{id}/sso/oidc — auth required, owner only ──
        var authed = app
            .MapGroup("/api/v1/organizations/{id}/sso")
            .RequireAuthorization()
            .WithTags("SSO");

        authed.MapPut("oidc", UpsertOidcConfig)
            .WithName("UpsertOidcConfig")
            .Accepts<OidcConfigRequest>("application/json")
            .Produces<SsoOrganizationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        // ── Public OIDC routes — no auth ──────────────────────────────────────
        var pub = app
            .MapGroup("/api/v1/sso/oidc")
            .WithTags("SSO");

        pub.MapGet("{orgId}/login", LoginRedirect)
            .WithName("OidcLoginRedirect")
            .RequireRateLimiting("oidc-login")
            .Produces(StatusCodes.Status302Found)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        pub.MapGet("{orgId}/callback", CallbackHandler)
            .WithName("OidcCallback")
            .RequireRateLimiting("oidc-callback")
            .Produces(StatusCodes.Status302Found)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    // ── Handlers ──────────────────────────────────────────────────────────────

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> UpsertOidcConfig(
        string id,
        [FromBody] OidcConfigRequest body,
        CurrentUserAccessor users,
        OidcService svc,
        CancellationToken ct)
    {
        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (dto, err, field, message) = await svc.UpsertOidcConfigAsync(userId.Value, orgGuid, body, ct);

        return err switch
        {
            SsoError.None => HttpResults.Ok(new SsoOrganizationResponse(
                dto!.Id, dto.Name, dto.Slug, dto.Role,
                dto.SeatCount, dto.SeatLimit, dto.Status, dto.CreatedAt,
                SsoEnabled: true, SsoProvider: "oidc")),
            SsoError.TierIneligible => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoTierIneligible, "SSO requires an Enterprise subscription."),
                statusCode: StatusCodes.Status402PaymentRequired),
            SsoError.InvalidConfig => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.InvalidSsoConfig, message ?? "Invalid SSO configuration.", field),
                statusCode: StatusCodes.Status400BadRequest),
            SsoError.OidcDiscoveryFailed => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.OidcDiscoveryFailed, message ?? "OIDC discovery failed."),
                statusCode: StatusCodes.Status400BadRequest),
            SsoError.PermissionDenied => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.PermissionDenied, message ?? "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> LoginRedirect(
        string orgId,
        OidcService svc,
        IOptions<OidcOptions> oidcOptions,
        CancellationToken ct)
    {
        if (!Guid.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (authorizeUrl, state, nonce, pkceVerifier, err, _) =
            await svc.BuildLoginAsync(orgGuid, ct);

        return err switch
        {
            SsoError.None => new OidcLoginResult(authorizeUrl!, state!, nonce!, pkceVerifier!, oidcOptions.Value),
            SsoError.TierIneligible => SsoNotFoundResult.Instance,
            SsoError.SsoNotEnabled or SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "SSO is not enabled for this organization."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> CallbackHandler(
        string orgId,
        [FromQuery] string? code,
        [FromQuery] string? state,
        HttpContext httpContext,
        OidcService svc,
        IOptions<OidcOptions> oidcOptions,
        IOptions<SamlOptions> samlOptions,
        CancellationToken ct)
    {
        if (!Guid.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        // Read state / nonce / pkce from the short-lived cookies
        var expectedState = httpContext.Request.Cookies[oidcOptions.Value.StateCookieName] ?? string.Empty;
        var expectedNonce = httpContext.Request.Cookies[oidcOptions.Value.NonceCookieName] ?? string.Empty;
        var pkceVerifier = httpContext.Request.Cookies[oidcOptions.Value.PkceCookieName] ?? string.Empty;

        var (sessionCookie, redirectUrl, err, message) = await svc.ConsumeCallbackAsync(
            orgGuid,
            code ?? string.Empty,
            state ?? string.Empty,
            expectedState,
            expectedNonce,
            pkceVerifier,
            ct);

        if (err == SsoError.None)
        {
            return new OidcCallbackResult(
                sessionCookie!, redirectUrl!, oidcOptions.Value, samlOptions.Value);
        }

        return err switch
        {
            SsoError.TierIneligible => SsoNotFoundResult.Instance,
            SsoError.OidcStateMismatch => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.OidcStateMismatch, message ?? "State mismatch."),
                statusCode: StatusCodes.Status400BadRequest),
            SsoError.OidcNonceMismatch => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.OidcNonceMismatch, message ?? "Nonce mismatch."),
                statusCode: StatusCodes.Status400BadRequest),
            SsoError.OidcInvalidIdToken => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.OidcInvalidIdToken, message ?? "Invalid id_token."),
                statusCode: StatusCodes.Status401Unauthorized),
            SsoError.OidcTokenExchangeFailed => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.OidcTokenExchangeFailed, message ?? "Token exchange failed."),
                statusCode: StatusCodes.Status401Unauthorized),
            SsoError.UserNotMember => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoUserNotMember, message ?? "User is not a member."),
                statusCode: StatusCodes.Status403Forbidden),
            SsoError.SsoNotEnabled or SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "SSO is not enabled or organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }
}

/// <summary>
/// Custom <see cref="IResult"/> that sets the OIDC state/nonce/pkce cookies and redirects to IdP.
/// </summary>
internal sealed class OidcLoginResult(
    string authorizeUrl,
    string state,
    string nonce,
    string pkceVerifier,
    OidcOptions options) : IResult
{
    /// <inheritdoc/>
    public Task ExecuteAsync(HttpContext httpContext)
    {
        var cookiePath = "/api/v1/sso/oidc";
        var cookieOptions = new CookieOptions
        {
            HttpOnly = true,
            Secure = false,  // HTTP allowed in dev; reverse proxy enforces HTTPS in prod
            SameSite = SameSiteMode.Lax,
            Path = cookiePath,
            MaxAge = options.StateCookieTtl,
        };

        httpContext.Response.Cookies.Append(options.StateCookieName, state, cookieOptions);
        httpContext.Response.Cookies.Append(options.NonceCookieName, nonce, cookieOptions);
        httpContext.Response.Cookies.Append(options.PkceCookieName, pkceVerifier, cookieOptions);
        httpContext.Response.Redirect(authorizeUrl);
        return Task.CompletedTask;
    }
}

/// <summary>
/// Custom <see cref="IResult"/> that clears the OIDC short-lived cookies, sets the session
/// cookie, and redirects to the web portal.
/// </summary>
internal sealed class OidcCallbackResult(
    string sessionCookie,
    string redirectUrl,
    OidcOptions oidcOptions,
    SamlOptions samlOptions) : IResult
{
    /// <inheritdoc/>
    public Task ExecuteAsync(HttpContext httpContext)
    {
        var clearOptions = new CookieOptions
        {
            HttpOnly = true,
            Secure = false,
            SameSite = SameSiteMode.Lax,
            Path = "/api/v1/sso/oidc",
            MaxAge = TimeSpan.Zero,
        };

        // Clear the short-lived OIDC cookies
        httpContext.Response.Cookies.Append(oidcOptions.StateCookieName, "", clearOptions);
        httpContext.Response.Cookies.Append(oidcOptions.NonceCookieName, "", clearOptions);
        httpContext.Response.Cookies.Append(oidcOptions.PkceCookieName, "", clearOptions);

        // Set the session cookie (same as SAML)
        httpContext.Response.Cookies.Append(samlOptions.SessionCookieName, sessionCookie, new CookieOptions
        {
            HttpOnly = true,
            Secure = false,
            SameSite = SameSiteMode.Lax,
            Path = "/",
            MaxAge = samlOptions.SessionTtl,
        });

        httpContext.Response.Redirect(redirectUrl);
        return Task.CompletedTask;
    }
}
