using ApiTool.Backend.Organizations;

namespace ApiTool.Backend.Tests.Organizations;

/// <summary>Verifies slug format validation rules.</summary>
public sealed class SlugValidatorTests
{
    [Theory]
    [InlineData("acme", true)]
    [InlineData("ac", true)]
    [InlineData("acme-corp", true)]
    [InlineData("ab12", true)]
    [InlineData("Acme", false)]       // uppercase
    [InlineData("-acme", false)]      // leading hyphen
    [InlineData("acme-", false)]      // trailing hyphen
    [InlineData("a", false)]          // too short (min 2)
    [InlineData("acme_corp", false)]  // underscore not allowed
    [InlineData("", false)]           // empty
    public void Validates_slug_format(string slug, bool expected)
    {
        var valid = SlugValidator.TryValidate(slug, out _);
        valid.Should().Be(expected, because: $"slug '{slug}' expected valid={expected}");
    }

    [Fact]
    public void Rejects_slug_exceeding_max_length()
    {
        // 101 lowercase letters — one over the 100-character maximum.
        var slug = new string('a', 101);
        var valid = SlugValidator.TryValidate(slug, out var error);
        valid.Should().BeFalse(because: "slugs longer than 100 characters should be rejected");
        error.Should().NotBeNullOrEmpty();
    }

    [Fact]
    public void Accepts_slug_at_max_length()
    {
        // Exactly 100 lowercase letters — on the boundary.
        var slug = new string('a', 100);
        var valid = SlugValidator.TryValidate(slug, out _);
        valid.Should().BeTrue(because: "a slug of exactly 100 characters should be accepted");
    }
}
