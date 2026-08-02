// Refs docs/SPECIFICATION.md:8568-8575 (signature verification, SHA-1 rejection, multi-secret).
using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.GitHub.Webhooks;

namespace ApiTool.Backend.Tests.GitHub.Webhooks;

/// <summary>
/// Unit tests for GithubWebhookSignatureVerifier HMAC-SHA256 verification.
/// </summary>
public sealed class GithubWebhookSignatureVerifierTests
{
    private static string ComputeSignatureHeader(string secret, string body)
    {
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
        var hash = hmac.ComputeHash(Encoding.UTF8.GetBytes(body));
        return $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";
    }

    private static readonly byte[] TestBody = Encoding.UTF8.GetBytes("""{"action":"created"}""");
    private const string Secret = "whsec_test";

    [Fact]
    public void Returns_true_for_correct_signature()
    {
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(TestBody);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret]);
        result.Should().BeTrue();
    }

    [Fact]
    public void Returns_false_for_wrong_signature()
    {
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes("wrong_secret"));
        var hash = hmac.ComputeHash(TestBody);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret]);
        result.Should().BeFalse();
    }

    [Fact]
    public void Returns_false_for_missing_prefix()
    {
        // Header without "sha256=" prefix
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(TestBody);
        var header = Convert.ToHexString(hash).ToLowerInvariant(); // missing sha256= prefix

        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret]);
        result.Should().BeFalse();
    }

    [Fact]
    public void Returns_false_for_non_hex_chars()
    {
        var header = "sha256=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx";
        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret]);
        result.Should().BeFalse();
    }

    [Fact]
    public void Returns_false_for_wrong_length()
    {
        // 32 hex chars (16 bytes) instead of 64 hex chars (32 bytes)
        var header = "sha256=deadbeefdeadbeefdeadbeefdeadbeef";
        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret]);
        result.Should().BeFalse();
    }

    [Fact]
    public void Returns_true_when_any_secret_matches_multi_secret_list()
    {
        const string newSecret = "whsec_new";
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(newSecret));
        var hash = hmac.ComputeHash(TestBody);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        // Old secret is first but new secret matches
        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, [Secret, newSecret]);
        result.Should().BeTrue();
    }

    [Fact]
    public void Returns_false_when_no_secret_matches()
    {
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes("wrong1"));
        var hash = hmac.ComputeHash(TestBody);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, ["noMatch1", "noMatch2"]);
        result.Should().BeFalse();
    }

    [Fact]
    public void Skips_empty_secrets_in_list()
    {
        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(TestBody);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        // Empty string secrets should be skipped
        var result = GithubWebhookSignatureVerifier.TryVerify(TestBody, header, ["", Secret, ""]);
        result.Should().BeTrue();
    }

    /// <summary>
    /// Behavior #10 regression guard: the verifier must operate on the RAW body bytes,
    /// not on a re-serialized form. A JSON body with non-canonical whitespace (e.g.,
    /// extra spaces around colons) will produce a different byte sequence when
    /// re-serialized via <see cref="System.Text.Json.JsonSerializer"/>. Signing over the
    /// raw bytes and verifying against the same raw bytes must succeed; verifying against
    /// re-serialized bytes must fail — confirming the test guards the right invariant.
    /// </summary>
    [Fact]
    public void Accepts_non_canonical_whitespace_body_signed_over_raw_bytes()
    {
        // Non-canonical whitespace: spaces around the colon and after the comma.
        const string nonCanonicalBody = """{ "action" : "created" , "x" : 1 }""";
        var rawBytes = Encoding.UTF8.GetBytes(nonCanonicalBody);

        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(rawBytes);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        // Verifying against the original raw bytes must succeed.
        var result = GithubWebhookSignatureVerifier.TryVerify(rawBytes, header, [Secret]);
        result.Should().BeTrue("the verifier must use raw bytes, not re-serialized JSON");
    }

    [Fact]
    public void Rejects_non_canonical_body_when_verified_against_re_serialized_bytes()
    {
        // Prove the test above is meaningful: re-serializing the body changes the bytes,
        // so the HMAC computed over the re-serialized form does NOT match the header that
        // was computed over the raw (non-canonical) bytes.
        const string nonCanonicalBody = """{ "action" : "created" , "x" : 1 }""";
        var rawBytes = Encoding.UTF8.GetBytes(nonCanonicalBody);

        using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(Secret));
        var hash = hmac.ComputeHash(rawBytes);
        var header = $"sha256={Convert.ToHexString(hash).ToLowerInvariant()}";

        // Re-serialize via JsonDocument (simulates what a naive implementation might do).
        var doc = System.Text.Json.JsonDocument.Parse(nonCanonicalBody);
        var reSerializedBytes = Encoding.UTF8.GetBytes(
            System.Text.Json.JsonSerializer.Serialize(doc));

        // The raw bytes and re-serialized bytes must differ — proving the test is meaningful.
        reSerializedBytes.Should().NotEqual(rawBytes,
            "re-serialization removes non-canonical whitespace, changing the byte sequence");

        // Verifying against re-serialized bytes must FAIL — the HMAC was over the raw body.
        var resultWithReSerializedBytes = GithubWebhookSignatureVerifier.TryVerify(reSerializedBytes, header, [Secret]);
        resultWithReSerializedBytes.Should().BeFalse(
            "a verifier that re-serializes before comparing would silently accept a tampered body");
    }
}
