using System.IdentityModel.Tokens.Jwt;
using System.Security.Claims;
using System.Text;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>Mints HS256 bearer tokens for integration tests.</summary>
public static class TestTokens
{
    /// <summary>
    /// Creates a signed JWT bearer token for the given user identity.
    /// Uses the same key/issuer/audience as <see cref="BackendFactory"/>.
    /// </summary>
    /// <param name="userId">The user id written to the <c>sub</c> claim.</param>
    /// <param name="email">The email written to the <c>email</c> claim.</param>
    /// <returns>The raw JWT string (without "Bearer " prefix).</returns>
    public static string Create(Guid userId, string email)
    {
        var key = new SymmetricSecurityKey(Encoding.UTF8.GetBytes(BackendFactory.TestSigningKey));
        var credentials = new SigningCredentials(key, SecurityAlgorithms.HmacSha256);

        var claims = new[]
        {
            new Claim(JwtRegisteredClaimNames.Sub, userId.ToString()),
            new Claim(JwtRegisteredClaimNames.Email, email),
        };

        var token = new JwtSecurityToken(
            issuer: BackendFactory.TestIssuer,
            audience: BackendFactory.TestAudience,
            claims: claims,
            expires: DateTime.UtcNow.AddHours(1),
            signingCredentials: credentials);

        return new JwtSecurityTokenHandler().WriteToken(token);
    }

    /// <summary>
    /// Creates a valid bearer token with a random new user id and the given email.
    /// </summary>
    public static (string token, Guid userId) CreateNew(string email = "user@example.com")
    {
        var userId = Guid.NewGuid();
        return (Create(userId, email), userId);
    }
}
