using System.Text.RegularExpressions;
using ApiTool.Backend.Compliance.Gdpr;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Unit tests for <see cref="AnonymisationToken"/> — pure, deterministic SHA-256 token derivation.
/// </summary>
public sealed class AnonymisationTokenTests
{
    [Fact]
    public void Compute_returns_deleted_user_prefix_and_eight_hex_chars()
    {
        var token = AnonymisationToken.Compute(Guid.NewGuid(), Guid.NewGuid());
        token.Should().StartWith("deleted-user-");
        token.Length.Should().Be(21, because: "'deleted-user-' (13) + 8 hex chars = 21");
    }

    [Fact]
    public void Compute_is_deterministic_same_inputs_same_output()
    {
        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        var first = AnonymisationToken.Compute(userId, orgId);
        var second = AnonymisationToken.Compute(userId, orgId);
        first.Should().Be(second);
    }

    [Fact]
    public void Compute_different_orgs_yield_different_tokens()
    {
        var userId = Guid.NewGuid();
        var orgA = Guid.NewGuid();
        var orgB = Guid.NewGuid();
        var tokenA = AnonymisationToken.Compute(userId, orgA);
        var tokenB = AnonymisationToken.Compute(userId, orgB);
        tokenA.Should().NotBe(tokenB);
    }

    [Fact]
    public void Compute_different_users_yield_different_tokens()
    {
        var orgId = Guid.NewGuid();
        var userA = Guid.NewGuid();
        var userB = Guid.NewGuid();
        var tokenA = AnonymisationToken.Compute(userA, orgId);
        var tokenB = AnonymisationToken.Compute(userB, orgId);
        tokenA.Should().NotBe(tokenB);
    }

    [Fact]
    public void Token_format_matches_regex_deleted_user_eight_hex()
    {
        var token = AnonymisationToken.Compute(Guid.NewGuid(), Guid.NewGuid());
        Regex.IsMatch(token, @"^deleted-user-[0-9a-f]{8}$")
            .Should().BeTrue($"token '{token}' must match 'deleted-user-[0-9a-f]{{8}}'");
    }
}
