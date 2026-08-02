using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Text;
using ApiTool.Backend.Auth;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Sso;

/// <summary>Mints short-lived HS256 JWT session tokens for use after a successful SSO login.</summary>
public sealed class SessionTokenIssuer(IOptions<JwtOptions> jwtOptions, TimeProvider clock)
{
    /// <summary>
    /// Creates a signed JWT for the given user, suitable for use as the value of the
    /// <c>apitool_session</c> HTTP-only cookie.
    /// </summary>
    /// <param name="userId">The internal user id (written to the <c>sub</c> claim).</param>
    /// <param name="email">The user's email (written to the <c>email</c> claim).</param>
    /// <param name="ttl">Token lifetime.</param>
    /// <returns>The raw JWT string.</returns>
    public string IssueForUser(Guid userId, string email, TimeSpan ttl)
    {
        var opts = jwtOptions.Value;
        var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(opts.SigningKey));
        var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);

        var now = clock.GetUtcNow().UtcDateTime;
        var claims = new[]
        {
            new Claim(JwtRegisteredClaimNames.Sub, userId.ToString()),
            new Claim(JwtRegisteredClaimNames.Email, email),
        };

        var token = new JwtSecurityToken(
            issuer: opts.Issuer,
            audience: opts.Audience,
            claims: claims,
            notBefore: now,
            expires: now.Add(ttl),
            signingCredentials: credentials);

        return new JwtSecurityTokenHandler().WriteToken(token);
    }
}
