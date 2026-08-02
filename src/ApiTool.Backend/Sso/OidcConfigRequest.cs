namespace ApiTool.Backend.Sso;

/// <summary>Request body for <c>PUT /api/v1/organizations/{id}/sso/oidc</c>.</summary>
/// <param name="IssuerUrl">The OIDC issuer URL. Required.</param>
/// <param name="ClientId">OAuth 2.0 client identifier. Required.</param>
/// <param name="ClientSecret">OAuth 2.0 client secret. Required. Stored in <c>sso_credentials</c>, not in settings JSON.</param>
/// <param name="RedirectUri">
/// Optional absolute callback URL. When omitted the service synthesises
/// <c>{OidcOptions.BackendBaseUrl}/api/v1/sso/oidc/{orgId}/callback</c>.
/// </param>
/// <param name="Scopes">Optional scope string. Defaults to <c>openid email profile</c>.</param>
public sealed record OidcConfigRequest(
    string? IssuerUrl,
    string? ClientId,
    string? ClientSecret,
    string? RedirectUri = null,
    string? Scopes = null);
