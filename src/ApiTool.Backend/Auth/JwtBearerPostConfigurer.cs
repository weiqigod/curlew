using System.IdentityModel.Tokens.Jwt;
using System.Text;
using ApiTool.Backend.Licensing.Tokens;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Post-configures <see cref="JwtBearerOptions"/> from <see cref="JwtOptions"/> so that
/// test overrides applied via <c>IOptions</c> are picked up correctly.
/// </summary>
public sealed class JwtBearerPostConfigurer(
    IOptions<JwtOptions> jwtOptions,
    IOptions<TokenIssuerOptions> tokenIssuerOptions,
    AccessTokenVerificationKeyCache verificationKeys)
    : IPostConfigureOptions<JwtBearerOptions>
{
    /// <inheritdoc/>
    public void PostConfigure(string? name, JwtBearerOptions options)
    {
        if (name != JwtBearerDefaults.AuthenticationScheme)
            return;

        var jwt = jwtOptions.Value;
        var tokenIssuer = tokenIssuerOptions.Value;
        var signingKey = jwt.SigningKey.Length > 0
            ? jwt.SigningKey
            : "placeholder-key-not-used-in-production-00000000";

        options.TokenValidationParameters = new TokenValidationParameters
        {
            ValidateIssuer = true,
            ValidIssuer = jwt.Issuer,
            ValidateAudience = true,
            ValidAudiences = [jwt.Audience, tokenIssuer.AccessAudience],
            ValidateIssuerSigningKey = true,
            IssuerSigningKey = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(signingKey)),
            ValidAlgorithms = [SecurityAlgorithms.HmacSha256, SecurityAlgorithms.EcdsaSha256],
            ValidTypes = ["JWT", "at+jwt"],
            ValidateLifetime = true,
            NameClaimType = "sub",
        };
        options.Events = new JwtBearerEvents
        {
            OnMessageReceived = async context =>
            {
                var rawToken = ReadBearerToken(context.Request);
                if (rawToken is null)
                    return;

                JwtSecurityToken token;
                try
                {
                    token = new JwtSecurityTokenHandler().ReadJwtToken(rawToken);
                }
                catch (Exception ex) when (ex is ArgumentException or SecurityTokenException)
                {
                    return; // The normal bearer validator will produce the 401 response.
                }

                if (!string.Equals(token.Header.Typ, "at+jwt", StringComparison.Ordinal))
                    return;

                if (!string.Equals(token.Header.Alg, SecurityAlgorithms.EcdsaSha256, StringComparison.Ordinal)
                    || string.IsNullOrWhiteSpace(token.Header.Kid))
                {
                    context.Fail("CLI access tokens must use ES256 and include a kid header.");
                    return;
                }

                context.Options.TokenValidationParameters.IssuerSigningKeys =
                    await verificationKeys.GetAsync(token.Header.Kid, context.HttpContext.RequestAborted);
            },
            OnChallenge = UnauthorizedResponseWriter.WriteAsync,
        };
    }

    private static string? ReadBearerToken(HttpRequest request)
    {
        var authorization = request.Headers.Authorization.ToString();
        const string prefix = "Bearer ";
        if (!authorization.StartsWith(prefix, StringComparison.OrdinalIgnoreCase))
            return null;

        var token = authorization[prefix.Length..].Trim();
        return token.Length == 0 ? null : token;
    }
}
