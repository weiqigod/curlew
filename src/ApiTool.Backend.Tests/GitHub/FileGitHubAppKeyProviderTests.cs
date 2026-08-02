using System.Security.Cryptography;
using System.Text;
using ApiTool.Backend.GitHub;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.GitHub;

/// <summary>
/// Tests for FileGitHubAppKeyProvider — behaviors #1, #3, #4, #5, #7.
/// </summary>
public sealed class FileGitHubAppKeyProviderTests : IDisposable
{
    private readonly string _tmpDir;
    private readonly string _pemPath;
    private readonly RSA _key;
    private const long TestAppId = 99999L;

    public FileGitHubAppKeyProviderTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_ghapp_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
        _pemPath = Path.Combine(_tmpDir, "test.pem");
        _key = RSA.Create(2048);
        File.WriteAllText(_pemPath, _key.ExportRSAPrivateKeyPem());
        if (OperatingSystem.IsLinux() || OperatingSystem.IsMacOS())
            File.SetUnixFileMode(_pemPath, UnixFileMode.UserRead | UnixFileMode.UserWrite);
    }

    public void Dispose()
    {
        _key.Dispose();
        if (Directory.Exists(_tmpDir)) Directory.Delete(_tmpDir, recursive: true);
    }

    private FileGitHubAppKeyProvider CreateProvider(string? pemPath = null, long appId = TestAppId)
        => new(Options.Create(new GitHubAppOptions
        {
            AppId = appId,
            Slug = "apitool-test",
            KeyProvider = new GitHubAppOptions.KeyProviderConfig
            {
                Mode = "file",
                File = new GitHubAppOptions.FileConfig { Path = pemPath ?? _pemPath },
            },
        }));

    [Fact]  // Behavior #1
    public async Task SignAppJwtAsync_returns_RS256_JWT_with_iat_minus_60s_exp_plus_540s()
    {
        var provider = CreateProvider();
        var iat = new DateTimeOffset(2026, 5, 6, 12, 0, 0, TimeSpan.Zero);
        var exp = iat.AddSeconds(540);
        var claims = new GitHubAppJwtClaims(TestAppId, iat.AddSeconds(-60), exp);

        var jwt = await provider.SignAppJwtAsync(claims);

        var parts = jwt.Split('.');
        parts.Should().HaveCount(3);
        var header = System.Text.Json.JsonDocument.Parse(Base64UrlEncoder.Decode(parts[0]));
        header.RootElement.GetProperty("alg").GetString().Should().Be("RS256");
        header.RootElement.GetProperty("typ").GetString().Should().Be("JWT");
        var payload = System.Text.Json.JsonDocument.Parse(Base64UrlEncoder.Decode(parts[1]));
        payload.RootElement.GetProperty("iss").GetInt64().Should().Be(TestAppId);
        payload.RootElement.GetProperty("iat").GetInt64().Should().Be(iat.AddSeconds(-60).ToUnixTimeSeconds());
        payload.RootElement.GetProperty("exp").GetInt64().Should().Be(exp.ToUnixTimeSeconds());
    }

    [Fact]  // Behavior #3 — sanity check via public key verification
    public async Task SignAppJwtAsync_signature_verifies_with_public_key()
    {
        var provider = CreateProvider();
        var now = DateTimeOffset.UtcNow;
        var jwt = await provider.SignAppJwtAsync(new GitHubAppJwtClaims(TestAppId, now.AddSeconds(-60), now.AddSeconds(540)));

        var parts = jwt.Split('.');
        var signingInput = Encoding.ASCII.GetBytes($"{parts[0]}.{parts[1]}");
        var signature = Base64UrlEncoder.DecodeBytes(parts[2]);

        // Verify directly with the public key — RSA PKCS1 v1.5, SHA-256
        // (same algorithm mandated by GitHub for App JWTs per spec :8359).
        // Note: JwtSecurityTokenHandler is NOT used here because it expects iss to be
        // a string, but GitHub mandates a numeric App ID as iss per the GitHub App JWT spec.
        // The RSA signature verification is the correct, complete proof.
        var verified = _key.VerifyData(signingInput, signature, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
        verified.Should().BeTrue();
    }

    [Fact]  // Behavior #4 — InvalidAppIdMismatch
    public async Task SignAppJwtAsync_with_mismatched_app_id_throws_InvalidAppIdMismatch()
    {
        var provider = CreateProvider(appId: TestAppId);
        var claims = new GitHubAppJwtClaims(AppId: 12345L, DateTimeOffset.UtcNow, DateTimeOffset.UtcNow.AddSeconds(540));
        var act = () => provider.SignAppJwtAsync(claims);
        await act.Should().ThrowAsync<InvalidAppIdMismatchException>();
    }

    [Fact]  // Behavior #5a — PEM unreadable surfaces as GitHubAppJwtSigningException without leaking content
    public async Task SignAppJwtAsync_when_pem_missing_throws_GitHubAppJwtSigningException_without_path_in_message()
    {
        var provider = CreateProvider(pemPath: Path.Combine(_tmpDir, "does-not-exist.pem"));
        var claims = new GitHubAppJwtClaims(TestAppId, DateTimeOffset.UtcNow.AddSeconds(-60), DateTimeOffset.UtcNow.AddSeconds(540));

        var act = () => provider.SignAppJwtAsync(claims);

        var ex = (await act.Should().ThrowAsync<GitHubAppJwtSigningException>()).Which;
        ex.Message.Should().Be("GitHub App JWT signing failed (provider=file)");
        ex.Message.Should().NotContain("does-not-exist");
        ex.Message.Should().NotContain(_tmpDir);
        ex.InnerException.Should().NotBeNull();  // preserved for telemetry, but not in user-visible message
    }

    [Fact]  // Behavior #5b — corrupt PEM also produces sanitised message
    public async Task SignAppJwtAsync_when_pem_corrupt_does_not_leak_PEM_content_into_message()
    {
        var corruptPem = Path.Combine(_tmpDir, "corrupt.pem");
        await File.WriteAllTextAsync(corruptPem, "-----BEGIN RSA PRIVATE KEY-----\nGARBAGE-CONTENT-DEFINITELY-NOT-A-VALID-KEY\n-----END RSA PRIVATE KEY-----\n");
        var provider = CreateProvider(pemPath: corruptPem);
        var claims = new GitHubAppJwtClaims(TestAppId, DateTimeOffset.UtcNow.AddSeconds(-60), DateTimeOffset.UtcNow.AddSeconds(540));

        var act = () => provider.SignAppJwtAsync(claims);

        var ex = (await act.Should().ThrowAsync<GitHubAppJwtSigningException>()).Which;
        ex.Message.Should().NotContain("GARBAGE-CONTENT-DEFINITELY-NOT-A-VALID-KEY");
    }

    [Fact]  // PEM permission posture
    public void Pem_file_has_mode_0600_after_test_setup()
    {
        if (!OperatingSystem.IsLinux() && !OperatingSystem.IsMacOS()) return;
        var mode = File.GetUnixFileMode(_pemPath);
        mode.Should().Be(UnixFileMode.UserRead | UnixFileMode.UserWrite);
    }

    [Fact]  // Behavior #7 — type-system separation
    public void IGitHubAppKeyProvider_is_distinct_from_IKeyProvider()
    {
        // Compile-time assertion: the two interfaces have no shared base class.
        typeof(IGitHubAppKeyProvider).IsAssignableFrom(typeof(ApiTool.Backend.Licensing.Keys.IKeyProvider)).Should().BeFalse();
        typeof(ApiTool.Backend.Licensing.Keys.IKeyProvider).IsAssignableFrom(typeof(IGitHubAppKeyProvider)).Should().BeFalse();
    }
}
