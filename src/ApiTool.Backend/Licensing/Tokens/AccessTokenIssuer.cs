// Refs docs/SPECIFICATION.md:7878-7891 (Access token 9-claim shape).
// Refs RFC 8725 §3.11 — typ header enforced strictly.
using ApiTool.Backend.Licensing.Keys;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Licensing.Tokens;

/// <summary>
/// Mints Access tokens (typ=<c>at+jwt</c>) carrying the 9-claim shape from spec :7878-7891.
/// Signed by the active <see cref="IKeyProvider"/> key using ES256.
/// Lifetime: 1 hour.
/// </summary>
public sealed class AccessTokenIssuer(
    IKeyProvider keyProvider,
    IOptions<TokenIssuerOptions> opts,
    TimeProvider clock)
{
    private readonly TokenIssuerOptions _opts = opts.Value;

    /// <summary>Mints an Access token carrying the 9-claim shape from spec :7878-7891.</summary>
    public async Task<string> IssueAsync(AccessTokenInput input, CancellationToken ct = default)
    {
        var now = clock.GetUtcNow();
        var exp = now.Add(_opts.AccessLifetime);
        var jti = Guid.NewGuid().ToString("N");

        var activeKid = await keyProvider.GetActiveKidAsync(ct);

        var header = new Dictionary<string, object?>
        {
            ["typ"] = "at+jwt",
            ["kid"] = activeKid,
        };

        var payload = new Dictionary<string, object?>
        {
            ["iss"]       = _opts.Issuer,
            ["aud"]       = _opts.AccessAudience,
            ["sub"]       = input.UserId.ToString(),
            ["exp"]       = exp.ToUnixTimeSeconds(),
            ["nbf"]       = now.ToUnixTimeSeconds(),
            ["iat"]       = now.ToUnixTimeSeconds(),
            ["jti"]       = jti,
            ["tier"]      = input.Tier,
            ["org_id"]    = input.OrgId?.ToString(),
            ["device_id"] = input.DeviceId.ToString(),
        };

        return await JwsBuilder.BuildAsync(keyProvider, header, payload, ct);
    }
}
