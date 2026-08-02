using ApiTool.Backend.Rbac.CustomRoles;

namespace ApiTool.Backend.Tests.Rbac;

/// <summary>Verifies the wire-format helpers for custom role identifiers.</summary>
public sealed class RoleIdTests
{
    [Fact]
    public void Round_trip_format_then_parse()
    {
        var original = Guid.NewGuid();
        var formatted = RoleId.Format(original);
        var parsed = RoleId.TryParse(formatted, out var result);

        parsed.Should().BeTrue();
        result.Should().Be(original);
    }

    [Fact]
    public void Format_produces_role_prefix_and_32_hex_chars()
    {
        var id = Guid.NewGuid();
        var formatted = RoleId.Format(id);

        formatted.Should().StartWith("role_");
        formatted.Length.Should().Be(5 + 32); // "role_" + 32 hex chars
    }

    [Theory]
    [InlineData("role_")]
    [InlineData("org_11111111111111111111111111111111")]
    [InlineData("")]
    [InlineData(null)]
    [InlineData("role_zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz")] // invalid hex
    [InlineData("role_12345")]                             // too short
    public void TryParse_rejects_invalid(string? v)
    {
        var result = RoleId.TryParse(v, out var id);

        result.Should().BeFalse();
        id.Should().Be(Guid.Empty);
    }

    [Fact]
    public void TryParse_accepts_lowercase_hex()
    {
        var guid = Guid.NewGuid();
        var lower = "role_" + guid.ToString("N").ToLowerInvariant();
        RoleId.TryParse(lower, out var parsed).Should().BeTrue();
        parsed.Should().Be(guid);
    }
}
