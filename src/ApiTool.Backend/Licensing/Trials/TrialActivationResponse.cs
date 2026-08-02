namespace ApiTool.Backend.Licensing.Trials;

/// <summary>
/// 200 response body for <c>POST /api/v1/trials/{feature}</c>.
/// Refs docs/SPECIFICATION.md:5800-5860 (on-demand activation).
/// </summary>
public sealed record TrialActivationResponse(
    string Feature,
    string Kind,
    DateTime GrantedAt,
    DateTime ExpiresAt,
    TokenTrio Tokens);

/// <summary>The re-minted token trio returned alongside a successful trial activation.</summary>
public sealed record TokenTrio(
    string LicenseJwt,
    string AccessToken,
    string RefreshToken);
