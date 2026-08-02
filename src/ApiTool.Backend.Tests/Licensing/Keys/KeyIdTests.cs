// Tests for KeyId static helpers.
// Refs docs/SPECIFICATION.md:7990-8090 (Signing-Key Storage, kid format).
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Licensing.Keys;

public sealed class KeyIdTests
{
    [Theory]
    [InlineData("prod-es256-202605-a3f4d2", true)]
    [InlineData("dev-es256-202605-abcdef", true)]
    [InlineData("a", true)]                                         // shortest valid
    [InlineData("PROD-ES256-202605-A3F4D2", false)]                 // uppercase rejected
    [InlineData("prod-es256-202605-a3f4d2/../../etc/passwd", false)] // path traversal
    [InlineData("../keys/secret", false)]
    [InlineData("", false)]
    [InlineData("dev-es256-202605-a3f4d-with-too-many-characters-after-the-suffix-aa", false)] // 65 chars
    public void IsValid_returns_expected(string kid, bool expected)
        => KeyId.IsValid(kid).Should().Be(expected);

    [Fact]
    public void Generate_with_dev_env_yields_kid_matching_format()
    {
        var clock = new FakeClock(new DateTimeOffset(2026, 5, 4, 0, 0, 0, TimeSpan.Zero));
        var kid = KeyId.Generate("dev", "es256", clock);
        kid.Should().MatchRegex(@"^dev-es256-202605-[a-f0-9]{6}$");
    }

    [Fact]
    public void Generate_two_consecutive_yields_distinct_kids()
    {
        var clock = new FakeClock(new DateTimeOffset(2026, 5, 4, 0, 0, 0, TimeSpan.Zero));
        var a = KeyId.Generate("dev", "es256", clock);
        var b = KeyId.Generate("dev", "es256", clock);
        a.Should().NotBe(b);
    }
}
