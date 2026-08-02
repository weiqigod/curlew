// Refs docs/SPECIFICATION.md:8413-8416 (signed-state token for dashboard-initiated install flow).
namespace ApiTool.Backend.Tests.GitHub.Installations;

using ApiTool.Backend.GitHub.Installations;

/// <summary>
/// Unit tests for the HMAC-SHA256 state token used in the GitHub App install-URL flow.
/// </summary>
public sealed class GithubInstallationStateTokenTests
{
    private const string TestKey = "test-signing-key-32chars-abcdefgh!";

    private static GithubInstallationStatePayload SamplePayload(DateTimeOffset? expiresAt = null) =>
        new(
            OrgId: Guid.NewGuid(),
            UserId: Guid.NewGuid(),
            ExpiresAt: expiresAt ?? DateTimeOffset.UtcNow.AddMinutes(15),
            Nonce: "abc123");

    [Fact]
    public void Mint_then_TryVerify_returns_original_payload()
    {
        var payload = SamplePayload();
        var token = GithubInstallationStateToken.Mint(payload, TestKey);
        var now = payload.ExpiresAt.AddMinutes(-5);

        var result = GithubInstallationStateToken.TryVerify(token, TestKey, now);

        result.Should().NotBeNull();
        result!.OrgId.Should().Be(payload.OrgId);
        result.UserId.Should().Be(payload.UserId);
        result.Nonce.Should().Be(payload.Nonce);
        result.ExpiresAt.Should().BeCloseTo(payload.ExpiresAt, TimeSpan.FromSeconds(1));
    }

    [Fact]
    public void TryVerify_returns_null_when_signature_does_not_match()
    {
        var payload = SamplePayload();
        var token = GithubInstallationStateToken.Mint(payload, TestKey);
        var now = payload.ExpiresAt.AddMinutes(-5);

        // Tamper with signing key
        var result = GithubInstallationStateToken.TryVerify(token, "wrong-key-also-32chars-abcdefg!", now);

        result.Should().BeNull();
    }

    [Fact]
    public void TryVerify_returns_null_when_payload_body_tampered()
    {
        var payload = SamplePayload();
        var token = GithubInstallationStateToken.Mint(payload, TestKey);
        var now = payload.ExpiresAt.AddMinutes(-5);

        // Replace the body (first segment before '.') with different content
        var parts = token.Split('.');
        parts[0] = Convert.ToBase64String(System.Text.Encoding.UTF8.GetBytes("""{"org_id":"00000000-0000-0000-0000-000000000000","user_id":"00000000-0000-0000-0000-000000000000","expires_at":"2099-01-01T00:00:00+00:00","nonce":"evil"}"""))
            .TrimEnd('=').Replace('+', '-').Replace('/', '_');
        var tampered = string.Join('.', parts);

        var result = GithubInstallationStateToken.TryVerify(tampered, TestKey, now);

        result.Should().BeNull();
    }

    [Fact]
    public void TryVerify_returns_null_when_token_is_expired()
    {
        var payload = SamplePayload(expiresAt: DateTimeOffset.UtcNow.AddMinutes(-1));
        var token = GithubInstallationStateToken.Mint(payload, TestKey);
        var now = DateTimeOffset.UtcNow; // now > expiresAt

        var result = GithubInstallationStateToken.TryVerify(token, TestKey, now);

        result.Should().BeNull();
    }

    [Fact]
    public void TryVerify_returns_null_when_token_is_malformed_no_separator()
    {
        var result = GithubInstallationStateToken.TryVerify("notavalidtoken", TestKey, DateTimeOffset.UtcNow);
        result.Should().BeNull();
    }

    [Fact]
    public void TryVerify_returns_null_when_token_is_malformed_invalid_base64url()
    {
        var result = GithubInstallationStateToken.TryVerify("!!!.???", TestKey, DateTimeOffset.UtcNow);
        result.Should().BeNull();
    }

    [Fact]
    public void Mint_produces_base64url_token_with_dot_separator()
    {
        var payload = SamplePayload();
        var token = GithubInstallationStateToken.Mint(payload, TestKey);

        // Must have exactly one dot separator
        token.Split('.').Should().HaveCount(2);

        // Must not contain standard base64 padding or non-URL-safe chars
        token.Should().NotContain("=");
        token.Should().NotContain("+");
        token.Should().NotContain("/");
    }

    [Fact]
    public void Two_mints_of_same_payload_produce_different_tokens_when_nonce_differs()
    {
        // When callers supply different nonces the tokens differ
        var p1 = new GithubInstallationStatePayload(Guid.NewGuid(), Guid.NewGuid(), DateTimeOffset.UtcNow.AddMinutes(15), "nonce1");
        var p2 = p1 with { Nonce = "nonce2" };

        var t1 = GithubInstallationStateToken.Mint(p1, TestKey);
        var t2 = GithubInstallationStateToken.Mint(p2, TestKey);

        t1.Should().NotBe(t2);
    }
}
