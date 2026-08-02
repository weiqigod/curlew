using ApiTool.Backend.Auth;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>Unit tests for <see cref="AuthTokenIssuer"/>.</summary>
public sealed class AuthTokenIssuerTests
{
    [Fact]
    public void Mint_password_reset_returns_prst_prefixed_48_char_token()
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.PasswordResetPrefix);
        plaintext.Should().StartWith("prst_");
        plaintext.Length.Should().Be(48);
        hash.Length.Should().Be(32);
    }

    [Fact]
    public void Mint_email_verification_returns_evtk_prefixed_48_char_token()
    {
        var (plaintext, hash) = AuthTokenIssuer.Mint(AuthTokenIssuer.EmailVerificationPrefix);
        plaintext.Should().StartWith("evtk_");
        plaintext.Length.Should().Be(48);
        hash.Length.Should().Be(32);
    }

    [Fact]
    public void Mint_produces_distinct_tokens_each_call()
    {
        var (a, _) = AuthTokenIssuer.Mint("prst_");
        var (b, _) = AuthTokenIssuer.Mint("prst_");
        a.Should().NotBe(b);
    }

    [Fact]
    public void Hash_is_deterministic_for_same_plaintext()
    {
        var h1 = AuthTokenIssuer.Hash("prst_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx");
        var h2 = AuthTokenIssuer.Hash("prst_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx");
        h1.Should().Equal(h2);
    }

    [Fact]
    public void Body_is_url_safe_base64()
    {
        var (plaintext, _) = AuthTokenIssuer.Mint("prst_");
        var body = plaintext["prst_".Length..];
        body.Should().MatchRegex("^[A-Za-z0-9_-]+$");
    }
}
