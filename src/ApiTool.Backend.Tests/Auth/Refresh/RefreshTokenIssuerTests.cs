// Tests for RefreshTokenIssuer — pure helper for minting opaque refresh tokens.
using ApiTool.Backend.Auth.Refresh;

namespace ApiTool.Backend.Tests.Auth.Refresh;

public sealed class RefreshTokenIssuerTests
{
    private readonly RefreshTokenIssuer _sut = new(TimeProvider.System);

    [Fact]
    public void Mint_plaintext_is_43_base64url_chars_without_padding()
    {
        var userId   = Guid.NewGuid();
        var deviceId = Guid.NewGuid();
        var familyId = Guid.NewGuid();
        var now      = DateTime.UtcNow;

        var (plaintext, _) = _sut.Mint(userId, deviceId, familyId, parentId: null,
            familyRootIssuedAt: now, clientIp: null, userAgent: null);

        plaintext.Should().MatchRegex(@"^[A-Za-z0-9_-]{43}$",
            because: "opaque refresh token must be 32-byte base64url (no padding)");
    }

    [Fact]
    public void Mint_row_token_hash_is_sha256_of_plaintext()
    {
        var now = DateTime.UtcNow;
        var (plaintext, row) = _sut.Mint(
            Guid.NewGuid(), Guid.NewGuid(), Guid.NewGuid(),
            parentId: null, familyRootIssuedAt: now, clientIp: "1.2.3.4", userAgent: "test-ua");

        var expectedHash = RefreshTokenIssuer.Hash(plaintext);
        row.TokenHash.Should().Equal(expectedHash);
    }

    [Fact]
    public void Hash_is_deterministic_sha256_with_32_byte_output()
    {
        var h1 = RefreshTokenIssuer.Hash("abc");
        var h2 = RefreshTokenIssuer.Hash("abc");
        h1.Should().Equal(h2, because: "hash must be deterministic");
        h1.Should().HaveCount(32, because: "SHA-256 output is 32 bytes");
    }

    [Fact]
    public void Hash_differs_for_different_plaintexts()
    {
        var h1 = RefreshTokenIssuer.Hash("token-aaa");
        var h2 = RefreshTokenIssuer.Hash("token-bbb");
        h1.Should().NotEqual(h2);
    }

    [Fact]
    public void Mint_lifetime_is_90_days_when_family_root_not_near_expiry()
    {
        // Family root issued today → expires at today + 365 days.
        // 90-day window is well within 365d from root, so new token = now + 90d.
        var now = DateTime.UtcNow;
        var (_, row) = _sut.Mint(
            Guid.NewGuid(), Guid.NewGuid(), Guid.NewGuid(),
            parentId: null, familyRootIssuedAt: now, clientIp: null, userAgent: null);

        var expectedExpiry = now.AddDays(90);
        // Allow ±5 seconds for test execution time.
        row.ExpiresAt.Should().BeCloseTo(expectedExpiry, TimeSpan.FromSeconds(5));
    }

    [Fact]
    public void Mint_lifetime_clamps_to_absolute_deadline_when_near_365_day_limit()
    {
        // Family root issued 350 days ago → absolute deadline is 15 days from now.
        // The 90-day window exceeds that, so ExpiresAt must be clamped to (rootIssuedAt + 365d).
        var now            = DateTime.UtcNow;
        var rootIssuedAt   = now.AddDays(-350);
        var absoluteDeadline = rootIssuedAt.AddDays(365); // ≈ 15 days in the future

        var (_, row) = _sut.Mint(
            Guid.NewGuid(), Guid.NewGuid(), Guid.NewGuid(),
            parentId: null, familyRootIssuedAt: rootIssuedAt, clientIp: null, userAgent: null);

        row.ExpiresAt.Should().BeCloseTo(absoluteDeadline, TimeSpan.FromSeconds(5));
    }

    [Fact]
    public void Mint_populates_issued_at_and_user_id_and_device_id_on_row()
    {
        var userId   = Guid.NewGuid();
        var deviceId = Guid.NewGuid();
        var familyId = Guid.NewGuid();
        var parentId = Guid.NewGuid();
        var before = DateTime.UtcNow;
        var (_, row) = _sut.Mint(userId, deviceId, familyId,
            parentId: parentId, familyRootIssuedAt: before, clientIp: "10.0.0.1", userAgent: "ua");
        var after = DateTime.UtcNow;

        row.UserId.Should().Be(userId);
        row.DeviceId.Should().Be(deviceId);
        row.FamilyId.Should().Be(familyId);
        row.ParentId.Should().Be(parentId);
        row.IssuedAt.Should().BeOnOrAfter(before.AddSeconds(-1));
        row.IssuedAt.Should().BeOnOrBefore(after.AddSeconds(1));
        row.LastUsedIp.Should().Be("10.0.0.1");
        row.UserAgent.Should().Be("ua");
    }
}
