// Refs docs/SPECIFICATION.md:7854-7876 (License JWT 17-claim shape).
// Refs RFC 8725 §3.11 — typ header enforced strictly.
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Trials;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Mints License JWTs with the 17-claim shape defined in docs/SPECIFICATION.md:7854-7876.
/// Token type is <c>license+jwt</c> per RFC 8725 §3.11.
/// Signed by the active <see cref="IKeyProvider"/> key using ES256.
/// Lifetime: 30 days; grace_until adds 14 more days for offline tolerance.
/// Trial fields are populated by <see cref="ITrialStateResolver"/> per spec :5806-5816.
/// </summary>
public sealed class LicenseTokenIssuer(
    IKeyProvider keyProvider,
    IOptions<TokenIssuerOptions> opts,
    TimeProvider clock,
    ITrialStateResolver trialResolver)
{
    private readonly TokenIssuerOptions _opts = opts.Value;

    /// <summary>Mints a License JWT carrying the 17-claim shape from spec :7854-7876.</summary>
    public async Task<string> IssueAsync(LicenseTokenInput input, CancellationToken ct = default)
    {
        var now = clock.GetUtcNow();
        var exp = now.Add(_opts.LicenseLifetime);
        var graceUntil = exp.Add(_opts.LicenseGrace);
        var jti = Guid.NewGuid().ToString("N");

        var activeKid = await keyProvider.GetActiveKidAsync(ct);

        // M16-006: resolve trial state and union trialing features into features[].
        var trial = await trialResolver.ResolveAsync(input.UserId, input.Tier, ct);
        var features = trial.TrialingFeatures.Count == 0
            ? input.Features
            : input.Features.Concat(trial.TrialingFeatures).Distinct().ToArray();

        var header = new Dictionary<string, object?>
        {
            ["typ"] = "license+jwt",
            ["kid"] = activeKid,
        };

        var payload = new Dictionary<string, object?>
        {
            ["iss"]           = _opts.Issuer,
            ["aud"]           = _opts.LicenseAudience,
            ["sub"]           = input.UserId.ToString(),
            ["exp"]           = exp.ToUnixTimeSeconds(),
            ["nbf"]           = now.ToUnixTimeSeconds(),
            ["iat"]           = now.ToUnixTimeSeconds(),
            ["jti"]           = jti,
            ["email"]         = input.Email,
            ["tier"]          = input.Tier,
            ["features"]      = features,
            ["request_limit"] = input.RequestLimit,
            ["org_id"]        = input.OrgId?.ToString(),
            ["org_role"]      = input.OrgRole,
            ["device_id"]     = input.DeviceId.ToString(),
            ["trial_state"]   = trial.TrialState,
            ["trial_expiry"]  = (object?)trial.TrialExpiryUnixSeconds,
            ["grace_until"]   = graceUntil.ToUnixTimeSeconds(),
        };

        return await JwsBuilder.BuildAsync(keyProvider, header, payload, ct);
    }
}
