using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Sso;

/// <summary>Registers the three SAML 2.0 SSO endpoints.</summary>
public static class SamlEndpoints
{
    /// <summary>Maps all SAML SSO endpoints onto the given route builder.</summary>
    public static IEndpointRouteBuilder MapSamlEndpoints(this IEndpointRouteBuilder app)
    {
        // ── PUT /api/v1/organizations/{id}/sso/saml — auth required, owner only ──
        var authed = app
            .MapGroup("/api/v1/organizations/{id}/sso")
            .RequireAuthorization()
            .WithTags("SSO");

        authed.MapGet("", GetSsoConfig)
            .WithName("GetSsoConfig")
            .Produces<SsoConfigView>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        authed.MapPut("saml", UpsertSamlConfig)
            .WithName("UpsertSamlConfig")
            .Accepts<SamlConfigRequest>("application/json")
            .Produces<SsoOrganizationResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        // ── Public SAML routes — no auth ──────────────────────────────────────
        var pub = app
            .MapGroup("/api/v1/sso/saml")
            .WithTags("SSO");

        pub.MapGet("{orgId}/login", LoginRedirect)
            .WithName("SamlLoginRedirect")
            .Produces(StatusCodes.Status302Found)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        pub.MapPost("{orgId}/acs", AcsCallback)
            .WithName("SamlAcsCallback")
            .DisableAntiforgery()  // public endpoint — no CSRF token; relies on SAML signature instead
            .Produces(StatusCodes.Status302Found)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    // ── Handlers ──────────────────────────────────────────────────────────────

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> GetSsoConfig(
        string id,
        CurrentUserAccessor users,
        SsoService svc,
        CancellationToken ct)
    {
        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (view, err) = await svc.GetConfigAsync(userId.Value, orgGuid, ct);

        return err switch
        {
            SsoError.None => HttpResults.Ok(view),
            SsoError.TierIneligible => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoTierIneligible, "SSO requires an Enterprise subscription."),
                statusCode: StatusCodes.Status402PaymentRequired),
            SsoError.PermissionDenied => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.PermissionDenied, "Only the organization owner may view SSO configuration."),
                statusCode: StatusCodes.Status403Forbidden),
            SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> UpsertSamlConfig(
        string id,
        [FromBody] SamlConfigRequest body,
        CurrentUserAccessor users,
        SsoService svc,
        CancellationToken ct)
    {
        if (!OrgId.TryParse(id, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        var (dto, err, field, message) = await svc.UpsertSamlConfigAsync(userId.Value, orgGuid, body, ct);

        return err switch
        {
            SsoError.None => HttpResults.Ok(new SsoOrganizationResponse(
                dto!.Id, dto.Name, dto.Slug, dto.Role,
                dto.SeatCount, dto.SeatLimit, dto.Status, dto.CreatedAt,
                SsoEnabled: true, SsoProvider: "saml")),
            SsoError.TierIneligible => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoTierIneligible, "SSO requires an Enterprise subscription."),
                statusCode: StatusCodes.Status402PaymentRequired),
            SsoError.InvalidConfig => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.InvalidSsoConfig, message ?? "Invalid SSO configuration.", field),
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
        SsoService svc,
        CancellationToken ct)
    {
        if (!Guid.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var (redirectUrl, err) = await svc.BuildLoginAsync(orgGuid, ct);

        return err switch
        {
            SsoError.None => HttpResults.Redirect(redirectUrl!),
            SsoError.TierIneligible => SsoNotFoundResult.Instance,
            SsoError.SsoNotEnabled => HttpResults.Json(
                new ErrorResponse("organization_not_found", "SSO is not enabled for this organization."),
                statusCode: StatusCodes.Status404NotFound),
            SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> AcsCallback(
        string orgId,
        [FromForm] string? samlResponse,
        SsoService svc,
        IOptions<SamlOptions> samlOptions,
        CancellationToken ct)
    {
        if (!Guid.TryParse(orgId, out var orgGuid))
            return HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound);

        var responseBase64 = samlResponse ?? string.Empty;

        var (sessionCookie, redirectUrl, err, message) = await svc.ConsumeAssertionAsync(orgGuid, responseBase64, ct);

        return err switch
        {
            SsoError.None => SetSessionCookieAndRedirect(sessionCookie!, redirectUrl!, samlOptions.Value),
            SsoError.TierIneligible => SsoNotFoundResult.Instance,
            SsoError.SignatureInvalid => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SamlSignatureInvalid, message ?? "SAML response validation failed."),
                statusCode: StatusCodes.Status401Unauthorized),
            SsoError.AssertionExpired => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.AssertionExpired, message ?? "SAML assertion has expired."),
                statusCode: StatusCodes.Status401Unauthorized),
            SsoError.UserNotMember => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoUserNotMember, message ?? "User is not a member of this organization."),
                statusCode: StatusCodes.Status403Forbidden),
            SsoError.SsoNotEnabled => HttpResults.Json(
                new ErrorResponse(SsoErrorCodes.SsoNotEnabled, "SSO is not enabled for this organization."),
                statusCode: StatusCodes.Status404NotFound),
            SsoError.OrgNotFound => HttpResults.Json(
                new ErrorResponse("organization_not_found", "Organization not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static IResult SetSessionCookieAndRedirect(string cookieValue, string redirectUrl, SamlOptions samlOptions)
    {
        return new SamlRedirectResult(cookieValue, redirectUrl, samlOptions.SessionCookieName, samlOptions.SessionTtl);
    }
}

/// <summary>
/// Custom <see cref="IResult"/> that sets an HTTP-only session cookie and redirects the browser.
/// </summary>
internal sealed class SamlRedirectResult(
    string cookieValue,
    string location,
    string cookieName,
    TimeSpan sessionTtl) : IResult
{
    /// <inheritdoc/>
    public Task ExecuteAsync(HttpContext httpContext)
    {
        httpContext.Response.Cookies.Append(cookieName, cookieValue, new CookieOptions
        {
            HttpOnly = true,
            Secure = false,  // allow HTTP in dev/test; reverse proxy enforces HTTPS in prod
            SameSite = SameSiteMode.Lax,
            Path = "/",
            MaxAge = sessionTtl,   // align browser cookie lifetime with the JWT TTL
        });
        httpContext.Response.Redirect(location);
        return Task.CompletedTask;
    }
}

/// <summary>
/// Custom <see cref="IResult"/> that returns HTTP 404 with no body and
/// <c>Cache-Control: no-store</c>. Used by the public SAML/OIDC flow endpoints
/// to mask non-Enterprise orgs the same way an unconfigured org appears (M15-002).
/// </summary>
internal sealed class SsoNotFoundResult : IResult
{
    /// <summary>Singleton instance — the result carries no per-request state.</summary>
    public static readonly SsoNotFoundResult Instance = new();

    /// <inheritdoc/>
    public Task ExecuteAsync(HttpContext httpContext)
    {
        httpContext.Response.StatusCode = StatusCodes.Status404NotFound;
        httpContext.Response.Headers.CacheControl = "no-store";
        httpContext.Response.ContentLength = 0;
        return Task.CompletedTask;
    }
}

/// <summary>
/// Response for successful SSO config upsert — extends org fields with SSO status.
/// </summary>
public sealed record SsoOrganizationResponse(
    string Id,
    string Name,
    string Slug,
    string Role,
    int SeatCount,
    int SeatLimit,
    string Status,
    DateTime CreatedAt,
    bool SsoEnabled,
    string SsoProvider);
