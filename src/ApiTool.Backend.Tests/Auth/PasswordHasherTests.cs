using ApiTool.Backend.Auth;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>Tests for Argon2id PHC-format password hasher.</summary>
public sealed class PasswordHasherTests
{
    [Theory]
    [InlineData("ChangeMe!ChangeMe!")]
    [InlineData("a-long-enough-password-123")]
    public void Hash_then_verify_returns_true(string password)
    {
        var h = new PasswordHasher();
        var encoded = h.Hash(password);
        encoded.Should().StartWith("$argon2id$v=19$m=65536,t=3,p=4$");
        h.Verify(password, encoded).Should().BeTrue();
    }

    [Fact]
    public void Verify_with_wrong_password_returns_false()
    {
        var h = new PasswordHasher();
        h.Verify("wrong-password-xyz", h.Hash("ChangeMe!ChangeMe!")).Should().BeFalse();
    }

    [Theory]
    [InlineData("")]
    [InlineData("not-a-phc-string")]
    [InlineData("$argon2id$v=19$m=foo,t=3,p=4$c2FsdA==$aGFzaA==")]
    [InlineData("$argon2d$v=19$m=65536,t=3,p=4$c2FsdA==$aGFzaA==")]
    [InlineData("$argon2id$v=19$m=65536,t=3,p=4$!!!not-base64!!!$aGFzaA==")]
    public void Verify_with_malformed_hash_returns_false(string encoded)
    {
        new PasswordHasher().Verify("ChangeMe!ChangeMe!", encoded).Should().BeFalse();
    }

    [Fact]
    public void Hash_produces_different_salts_for_same_password()
    {
        var h = new PasswordHasher();
        var a = h.Hash("same-password-xyz");
        var b = h.Hash("same-password-xyz");
        a.Should().NotBe(b);
    }
}
