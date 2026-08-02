using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Organizations;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Sso;

/// <summary>Business logic for SAML 2.0 SSO: config upsert, login redirect, and ACS assertion consumption.</summary>
public sealed class SsoService(
    AppDbContext db,
    ISamlHandler saml,
    SessionTokenIssuer sessions,
    TimeProvider clock,
    IOptions<SamlOptions> options,
    IAuditWriter audit)
{
    /// <summary>
    /// Upserts the SAML configuration for an organisation. Only the owner may call this.
    /// </summary>
    /// <param name="userId">The requesting user's id (must be the org owner).</param>
    /// <param name="orgId">Target organisation.</param>
    /// <param name="req">The new configuration.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple: (dto, error, field, message). On success error is <see cref="SsoError.None"/> and
    /// dto is the updated organisation; on failure dto is <see langword="null"/>.
    /// </returns>
    public async Task<(OrganizationDto? dto, SsoError err, string? field, string? message)>
        UpsertSamlConfigAsync(Guid userId, Guid orgId, SamlConfigRequest req, CancellationToken ct)
    {
        // Tier gate (M15-002) — must run before any other validation so non-Enterprise
        // tenants can never even probe SSO config shape.
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, tierErr, null, "SSO requires an Enterprise subscription.");

        // Validate required fields
        if (string.IsNullOrWhiteSpace(req.IdpMetadataUrl))
            return (null, SsoError.InvalidConfig, "idp_metadata_url", "idp_metadata_url is required.");
        if (string.IsNullOrWhiteSpace(req.AcsUrl))
            return (null, SsoError.InvalidConfig, "acs_url", "acs_url is required.");
        if (string.IsNullOrWhiteSpace(req.EntityId))
            return (null, SsoError.InvalidConfig, "entity_id", "entity_id is required.");

        // RBAC: owner only
        var membership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (membership is null || membership.Role != OrgRole.Owner)
            return (null, SsoError.PermissionDenied, null, "Only the organization owner may configure SSO.");

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, SsoError.OrgNotFound, null, "Organization not found.");

        // Deserialize existing settings, update SSO keys, re-serialize
        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        settings.SsoEnabled = true;
        settings.SsoProvider = "saml";
        settings.SsoConfig = new SsoConfig(
            req.IdpMetadataUrl,
            req.AcsUrl,
            req.EntityId,
            req.IdpSsoUrl,
            // strip cert from config blob — it lives in sso_credentials
            null);

        var now = clock.GetUtcNow().UtcDateTime;
        org.SettingsJson = settings.ToJson();
        org.UpdatedAt = now;

        // Persist IdP cert in sso_credentials if provided
        if (!string.IsNullOrWhiteSpace(req.IdpCertPem))
        {
            var kid = ComputeKid(req.IdpCertPem);
            var existing = await db.SsoCredentials
                .FirstOrDefaultAsync(c => c.OrgId == orgId && c.Kid == kid, ct);
            if (existing is null)
            {
                db.SsoCredentials.Add(new SsoCredential
                {
                    Id = Guid.NewGuid(),
                    OrgId = orgId,
                    Kid = kid,
                    PublicCertPem = req.IdpCertPem,
                    CreatedAt = now,
                });
            }
            else
            {
                existing.PublicCertPem = req.IdpCertPem;
            }
        }

        audit.Append(new AuditEvent(
            OrgId: orgId,
            ActorId: userId,
            EventType: "sso.config_updated",
            Payload: new { provider = "saml" }));

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
    /// Reads the SSO configuration for the given organisation.
    /// Secrets (IdP cert, OIDC client_secret) are never returned.
    /// Requires the caller to be the organisation owner.
    /// </summary>
    /// <param name="userId">The requesting user's id (must be the org owner).</param>
    /// <param name="orgId">Target organisation.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple: (view, error). On success error is <see cref="SsoError.None"/> and view is populated;
    /// on failure view is <see langword="null"/>.
    /// </returns>
    public async Task<(SsoConfigView? view, SsoError err)>
        GetConfigAsync(Guid userId, Guid orgId, CancellationToken ct)
    {
        // Tier gate (M15-002).
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, tierErr);

        var membership = await db.OrganizationMembers
            .FirstOrDefaultAsync(m => m.OrgId == orgId && m.UserId == userId, ct);
        if (membership is null || membership.Role != OrgRole.Owner)
            return (null, SsoError.PermissionDenied);

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null) return (null, SsoError.OrgNotFound);

        var settings = OrganizationSettings.FromJson(org.SettingsJson);

        var samlView = settings.SsoConfig is null ? null : new SamlConfigView(
            settings.SsoConfig.IdpMetadataUrl,
            settings.SsoConfig.AcsUrl,
            settings.SsoConfig.EntityId,
            settings.SsoConfig.IdpSsoUrl);

        var oidcView = settings.OidcConfig is null ? null : new OidcConfigView(
            settings.OidcConfig.IssuerUrl,
            settings.OidcConfig.ClientId,
            settings.OidcConfig.RedirectUri,
            settings.OidcConfig.Scopes);

        var view = new SsoConfigView(
            SsoEnabled: settings.SsoEnabled,
            SsoProvider: settings.SsoProvider,
            SamlConfig: samlView,
            OidcConfig: oidcView);

        return (view, SsoError.None);
    }

    /// <summary>
    /// Builds the IdP login redirect URL for an organisation.
    /// Returns <see cref="SsoError.SsoNotEnabled"/> if SSO is not configured.
    /// </summary>
    /// <param name="orgId">Target organisation.</param>
    /// <param name="ct">Cancellation token.</param>
    public async Task<(string? redirectUrl, SsoError err)>
        BuildLoginAsync(Guid orgId, CancellationToken ct)
    {
        // Tier gate (M15-002).
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, tierErr);

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, SsoError.OrgNotFound);

        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        if (!settings.SsoEnabled || settings.SsoConfig is null)
            return (null, SsoError.SsoNotEnabled);

        var relayState = orgId.ToString("N");
        var result = saml.BuildAuthnRequest(settings.SsoConfig, orgId, relayState);
        // NOTE: result.RequestId is intentionally not stored. InResponseTo validation
        // (replay-attack protection) was identified in the plan's risk section but is
        // not required by the M5-001 behaviors list. It is deferred to a follow-up task
        // rather than implemented here, to avoid scope creep on the M5-001 slice.
        return (result.RedirectUrl, SsoError.None);
    }

    /// <summary>
    /// Validates a SAMLResponse and issues a session cookie for the matched org member.
    /// </summary>
    /// <param name="orgId">Target organisation.</param>
    /// <param name="samlResponseBase64">Base64-encoded SAMLResponse from the form post.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>
    /// Tuple: (sessionCookie, redirectUrl, error, message). On success the cookie value and
    /// portal redirect URL are populated; on failure both are <see langword="null"/>.
    /// </returns>
    public async Task<(string? sessionCookie, string? redirectUrl, SsoError err, string? message)>
        ConsumeAssertionAsync(Guid orgId, string samlResponseBase64, CancellationToken ct)
    {
        // Tier gate (M15-002).
        var tierErr = await SsoTierGate.EnsureEnterpriseAsync(db, orgId, ct);
        if (tierErr != SsoError.None)
            return (null, null, tierErr, "SSO requires an Enterprise subscription.");

        var org = await db.Organizations.FirstOrDefaultAsync(o => o.Id == orgId, ct);
        if (org is null)
            return (null, null, SsoError.OrgNotFound, "Organization not found.");

        var settings = OrganizationSettings.FromJson(org.SettingsJson);
        if (!settings.SsoEnabled || settings.SsoConfig is null)
            return (null, null, SsoError.SsoNotEnabled, "SSO is not enabled for this organization.");

        // Load most-recent IdP cert from sso_credentials
        var cred = await db.SsoCredentials
            .Where(c => c.OrgId == orgId)
            .OrderByDescending(c => c.CreatedAt)
            .FirstOrDefaultAsync(ct);

        var certPem = cred?.PublicCertPem ?? settings.SsoConfig.IdpCertPem ?? string.Empty;

        var now = clock.GetUtcNow().UtcDateTime;
        var validation = saml.ValidateResponse(settings.SsoConfig, samlResponseBase64, certPem, now);

        if (!validation.Success)
        {
            var failureReason = validation.ErrorCode ?? "validation_failed";
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: Guid.Empty,
                EventType: "sso.login",
                Success: false, FailureReason: failureReason,
                ActorEmail: validation.Email));
            await db.SaveChangesAsync(ct);
            var failErr = validation.ErrorCode == SsoErrorCodes.AssertionExpired
                ? SsoError.AssertionExpired
                : SsoError.SignatureInvalid;
            var failMsg = validation.ErrorCode == SsoErrorCodes.AssertionExpired
                ? "SAML assertion has expired."
                : "SAML response validation failed.";
            return (null, null, failErr, failMsg);
        }

        var email = validation.Email!;

        // Look up org member by email
        var user = await db.Users.FirstOrDefaultAsync(u => u.Email == email, ct);
        if (user is null)
        {
            audit.Append(new AuditEvent(
                OrgId: orgId, ActorId: Guid.Empty,
                EventType: "sso.login",
                Success: false, FailureReason: "user_not_found",
                ActorEmail: email));
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
                Success: false, FailureReason: "not_member",
                ActorEmail: email));
            await db.SaveChangesAsync(ct);
            return (null, null, SsoError.UserNotMember, "User is not a member of this organization.");
        }

        var sessionCookie = sessions.IssueForUser(user.Id, email, options.Value.SessionTtl);
        audit.Append(new AuditEvent(
            OrgId: orgId, ActorId: user.Id,
            EventType: "sso.login",
            Success: true,
            ActorEmail: email));
        await db.SaveChangesAsync(ct);

        return (sessionCookie, options.Value.WebPortalUrl, SsoError.None, null);
    }

    /// <summary>Computes a stable key identifier (KID) as the hex SHA-256 of the PEM string bytes.</summary>
    private static string ComputeKid(string pem)
    {
        var bytes = SHA256.HashData(Encoding.UTF8.GetBytes(pem));
        return Convert.ToHexString(bytes).ToLowerInvariant()[..32];  // first 32 hex chars = 128-bit prefix
    }
}
