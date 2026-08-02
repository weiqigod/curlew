using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Sso;

/// <summary>
/// Business logic for OIDC SSO: config upsert, login redirect generation, and callback handling.
/// </summary>
public sealed class OidcService(
    AppDbContext db,
    IOidcHandler handler,
    SessionTokenIssuer sessions,
    TimeProvider clock,
    IOptions<OidcOptions> oidcOptions,
    IOptions<SamlOptions> samlOptions,  // shares WebPortalUrl, SessionCookieName, SessionTtl
    IAuditWriter audit)
{
    /// <summary>
    /// Upserts the OIDC configuration for an organisation. Only the owner may call this.
    /// Performs an eager discovery probe on success — returns
    /// <see cref="SsoError.OidcDiscoveryFailed"/> if the issuer URL is unreachable.
    /// </summary>
    public async Task<(OrganizationDto? dto, SsoError err, string? field, string? message)>
        UpsertOidcConfigAsync(Guid userId, Guid orgId, OidcConfigRequest req, CancellationToken ct)
    {
        // Tier gate (M15-002) — must run before any other validation.
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, tierErr, null, "SSO requires an Enterprise subscription.");

        // Validate required fields
        if (string.IsNullOrWhiteSpace(req.IssuerUrl))
            return (null, SsoError.InvalidConfig, "issuer_url", "issuer_url is required.");
        if (string.IsNullOrWhiteSpace(req.ClientId))
            return (null, SsoError.InvalidConfig, "client_id", "client_id is required.");
        if (string.IsNullOrWhiteSpace(req.ClientSecret))
            return (null, SsoError.InvalidConfig, "client_secret", "client_secret is required.");

        // RBAC: owner only
        var membership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (membership is null || membership.Role != OrgRole.Owner)
            return (null, SsoError.PermissionDenied, null, "Only the organization owner may configure SSO.");

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, SsoError.OrgNotFound, null, "Organization not found.");

        // Synthesise redirect URI if omitted
        var redirectUri = string.IsNullOrWhiteSpace(req.RedirectUri)
            ? $"{oidcOptions.Value.BackendBaseUrl}/api/v1/sso/oidc/{orgId}/callback"
            : req.RedirectUri;

        var config = new OidcConfig(
            req.IssuerUrl,
            req.ClientId,
            redirectUri,
            req.Scopes ?? "openid email profile");

        // Eager discovery probe (behaviour #2 in task YAML)
        var probeState = Guid.NewGuid().ToString("N");
        try
        {
            await handler.BuildAuthorizeAsync(config, probeState, probeState, probeState, ct);
        }
        catch (OidcDiscoveryException)
        {
            return (null, SsoError.OidcDiscoveryFailed, null, "OIDC discovery failed for the given issuer URL.");
        }

        // Update organisation settings — clear SAML config, set OIDC config
        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        settings.SsoEnabled = true;
        settings.SsoProvider = "oidc";
        settings.OidcConfig = config;
        settings.SsoConfig = null;

        var now = clock.GetUtcNow().UtcDateTime;
        org.SettingsJson = settings.ToJson();
        org.UpdatedAt = now;

        // Persist client secret in sso_credentials (reuse PublicCertPem column; KID = "oidc_client_secret")
        var existing = await db.SsoCredentials
            .FirstOrDefaultAsync(c => c.OrgId == orgId && c.Kid == "oidc_client_secret", ct);
        if (existing is null)
        {
            db.SsoCredentials.Add(new SsoCredential
            {
                Id = Guid.NewGuid(),
                OrgId = orgId,
                Kid = "oidc_client_secret",
                PublicCertPem = req.ClientSecret,
                CreatedAt = now,
            });
        }
        else
        {
            existing.PublicCertPem = req.ClientSecret;
        }

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "sso.config_updated",
            Payload: new { provider = "oidc" }));

        await db.SaveChangesAsync(ct);

        var seatCount = await db.OrganizationMembers.CountAsync(m => m.OrgId == orgId, ct);
        var seatLimit = await db.Subscriptions
            .Where(s => s.OrgId == orgId)
            .Select(s => (int?)s.SeatLimit)
            .FirstOrDefaultAsync(ct) ?? OrganizationService.FreeTierSeatLimit;

        var dto = new OrganizationDto(
            Id: OrgId.Format(org.Id),
            Name: org.Name,
			Slug: org.Slug,
			Role: "owner",
			Tier: "enterprise",
			SeatCount: seatCount,
            SeatLimit: seatLimit,
            Status: OrganizationService.ToStatusString(org.Status),
            CreatedAt: org.CreatedAt);

        return (dto, SsoError.None, null, null);
    }

    /// <summary>
    /// Builds the OIDC authorize URL for an organisation and returns the state, nonce, and
    /// PKCE verifier that the endpoint must persist in short-lived cookies.
    /// </summary>
    public async Task<(string? authorizeUrl, string? state, string? nonce, string? pkceVerifier, SsoError err, string? message)>
        BuildLoginAsync(Guid orgId, CancellationToken ct)
    {
        // Tier gate (M15-002).
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, null, null, null, tierErr, "SSO requires an Enterprise subscription.");

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, null, null, null, SsoError.OrgNotFound, "Organization not found.");

        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        if (!settings.SsoEnabled || settings.SsoProvider != "oidc" || settings.OidcConfig is null)
            return (null, null, null, null, SsoError.SsoNotEnabled, "OIDC SSO is not enabled for this organization.");

        // Generate cryptographically random state / nonce / PKCE verifier (32 bytes each)
        var state = GenerateRandom();
        var nonce = GenerateRandom();
        var pkceVerifier = GenerateRandom();

        OidcAuthorizeRequest result;
        try
        {
            result = await handler.BuildAuthorizeAsync(settings.OidcConfig, state, nonce, pkceVerifier, ct);
        }
        catch (OidcDiscoveryException)
        {
            return (null, null, null, null, SsoError.OidcDiscoveryFailed, "OIDC discovery failed.");
        }

        return (result.AuthorizeUrl, state, nonce, pkceVerifier, SsoError.None, null);
    }

    /// <summary>
    /// Validates the OIDC callback, exchanges the code for an id_token, and issues a session cookie.
    /// </summary>
    /// <param name="orgId">Target organisation.</param>
    /// <param name="code">The authorization code from the IdP callback.</param>
    /// <param name="receivedState">State parameter received from the IdP callback.</param>
    /// <param name="expectedState">State value stored in the browser cookie.</param>
    /// <param name="expectedNonce">Nonce value stored in the browser cookie.</param>
    /// <param name="pkceVerifier">PKCE verifier stored in the browser cookie.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(string? sessionCookie, string? redirectUrl, SsoError err, string? message)>
        ConsumeCallbackAsync(
            Guid orgId,
            string code,
            string receivedState,
            string expectedState,
            string expectedNonce,
            string pkceVerifier,
            CancellationToken ct)
    {
        // Tier gate (M15-002).
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, null, tierErr, "SSO requires an Enterprise subscription.");

        // Validate org exists before any audit writes (FK constraint on OrganizationAuditLog.OrgId).
        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, null, SsoError.OrgNotFound, "Organization not found.");

        // CSRF protection: state must match
        if (receivedState != expectedState)
        {
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: Guid.Empty,
                EventType: "sso.login",
                Success: false, FailureReason: SsoErrorCodes.OidcStateMismatch));
            await db.SaveChangesAsync(ct);
            return (null, null, SsoError.OidcStateMismatch, "OIDC state parameter mismatch.");
        }

        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        if (!settings.SsoEnabled || settings.SsoProvider != "oidc" || settings.OidcConfig is null)
            return (null, null, SsoError.SsoNotEnabled, "OIDC SSO is not enabled for this organization.");

        // Load client secret from sso_credentials
        var cred = await db.SsoCredentials
            .FirstOrDefaultAsync(c => c.OrgId == orgId && c.Kid == "oidc_client_secret", ct);
        var clientSecret = cred?.PublicCertPem ?? string.Empty;

        var now = clock.GetUtcNow().UtcDateTime;
        var validation = await handler.ExchangeAndValidateAsync(
            settings.OidcConfig, clientSecret, code, pkceVerifier, expectedNonce, now, ct);

        if (!validation.Success)
        {
            var failureReason = validation.ErrorCode ?? "validation_failed";
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: Guid.Empty,
                EventType: "sso.login",
                Success: false, FailureReason: failureReason));
            await db.SaveChangesAsync(ct);

            return validation.ErrorCode switch
            {
                SsoErrorCodes.OidcStateMismatch =>
                    (null, null, SsoError.OidcStateMismatch, "OIDC state parameter mismatch."),
                SsoErrorCodes.OidcNonceMismatch =>
                    (null, null, SsoError.OidcNonceMismatch, "OIDC nonce mismatch."),
                SsoErrorCodes.OidcInvalidIdToken =>
                    (null, null, SsoError.OidcInvalidIdToken, "OIDC id_token validation failed."),
                SsoErrorCodes.OidcTokenExchangeFailed =>
                    (null, null, SsoError.OidcTokenExchangeFailed, "OIDC token exchange failed."),
                _ =>
                    (null, null, SsoError.OidcInvalidIdToken, "OIDC validation failed."),
            };
        }

        var email = validation.Email!;

        // Look up org member by email
        var user = await db.Users.FirstOrDefaultAsync(u => u.Email == email, ct);
        if (user is null)
        {
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: Guid.Empty,
                EventType: "sso.login",
                Success: false, FailureReason: "user_not_found"));
            await db.SaveChangesAsync(ct);
            return (null, null, SsoError.UserNotMember, "User is not a member of this organization.");
        }

        var isMember = await db.OrganizationMembers
            .AnyAsync(m => m.OrgId == orgId && m.UserId == user.Id, ct);
        if (!isMember)
        {
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: user.Id,
                EventType: "sso.login",
                Success: false, FailureReason: "not_member"));
            await db.SaveChangesAsync(ct);
            return (null, null, SsoError.UserNotMember, "User is not a member of this organization.");
        }

        var sessionCookie = sessions.IssueForUser(user.Id, email, samlOptions.Value.SessionTtl);
        audit.Append(new AuditEvent(
            OrgId: orgId, ActorId: user.Id,
            EventType: "sso.login",
            Success: true));
        await db.SaveChangesAsync(ct);

        return (sessionCookie, samlOptions.Value.WebPortalUrl, SsoError.None, null);
    }

    /// <summary>Generates 32 cryptographically random bytes as a base64url string.</summary>
    private static string GenerateRandom()
    {
        var bytes = new byte[32];
        RandomNumberGenerator.Fill(bytes);
        return Convert.ToBase64String(bytes)
            .Replace('+', '-').Replace('/', '_').TrimEnd('=');  // base64url
    }
}
