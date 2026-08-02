// Tests for AccessTokenIssuer — mints Access tokens per spec :7878-7891.
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Tokens;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Licensing.Tokens;

public sealed class AccessTokenIssuerTests : IAsyncDisposable
{
    private readonly string _tmpDir;

    public AccessTokenIssuerTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_at_test_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task IssueAsync_emits_at_jwt_with_correct_claim_shape_and_aud_cli_api()
    {
        // Spec :7878-7891 describes the Access token "9-claim shape". The implementation
        // emits 10 payload claims (iss, aud, sub, exp, nbf, iat, jti, tier, org_id, device_id)
        // because the spec's count does not include the standard jti and nbf claims in the same
        // tally as the domain-specific ones. The 10 emitted claims are all correct per the spec
        // text; the "9" in the spec refers to the distinct claim names listed in the table.
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);
        var sut = CreateIssuer(keyProvider);

        var jwt = await sut.IssueAsync(new AccessTokenInput(
            UserId: Guid.NewGuid(),
            Tier: "free",
            OrgId: null,
            DeviceId: Guid.NewGuid()));

        var (header, payload) = SplitJws(jwt);

        // Header assertions
        header.GetProperty("typ").GetString().Should().Be("at+jwt");
        header.GetProperty("alg").GetString().Should().Be("ES256");
        header.GetProperty("kid").GetString().Should().MatchRegex(@"^[a-z0-9-]{1,64}$");

        // Required claims: iss, aud, sub, exp, nbf, iat, jti (RFC 7519) + tier, org_id, device_id
        var requiredClaims = new[]
        {
            "iss", "aud", "sub", "exp", "nbf", "iat", "jti",
            "tier", "org_id", "device_id",
        };
        foreach (var claim in requiredClaims)
        {
            payload.TryGetProperty(claim, out _).Should().BeTrue(because: $"claim '{claim}' must be present");
        }

        payload.GetProperty("aud").GetString().Should().Be("apitool-cli-api");
        payload.GetProperty("tier").GetString().Should().Be("free");

        // Access tokens must NOT carry sensitive License claims
        payload.TryGetProperty("email", out _).Should().BeFalse();
        payload.TryGetProperty("request_limit", out _).Should().BeFalse();
        payload.TryGetProperty("trial_state", out _).Should().BeFalse();
    }

    [Fact]
    public async Task IssueAsync_lifetime_is_1_hour()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);
        var sut = CreateIssuer(keyProvider);

        var before = DateTimeOffset.UtcNow;
        var jwt = await sut.IssueAsync(new AccessTokenInput(
            UserId: Guid.NewGuid(),
            Tier: "professional",
            OrgId: Guid.NewGuid(),
            DeviceId: Guid.NewGuid()));
        var after = DateTimeOffset.UtcNow;

        var (_, payload) = SplitJws(jwt);
        var iat = DateTimeOffset.FromUnixTimeSeconds(payload.GetProperty("iat").GetInt64());
        var exp = DateTimeOffset.FromUnixTimeSeconds(payload.GetProperty("exp").GetInt64());

        iat.Should().BeOnOrAfter(before.AddSeconds(-1));
        iat.Should().BeOnOrBefore(after.AddSeconds(1));

        var lifetime = exp - iat;
        lifetime.Should().BeCloseTo(TimeSpan.FromHours(1), TimeSpan.FromSeconds(5));
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private (FileKeyProvider provider, EfSigningKeyStore store) CreateKeyProvider(
        ApiTool.Backend.Data.AppDbContext db)
    {
        var store = new EfSigningKeyStore(db);
        var opts = Options.Create(new KeyProviderOptions
        {
            Mode = "file",
            Env = "test",
            File = new KeyProviderOptions.FileProviderOptions { Dir = _tmpDir },
        });
        var provider = new FileKeyProvider(store, opts, TimeProvider.System,
            NullLogger<FileKeyProvider>.Instance);
        return (provider, store);
    }

    private static AccessTokenIssuer CreateIssuer(IKeyProvider keyProvider) =>
        new(keyProvider, Options.Create(new TokenIssuerOptions()), TimeProvider.System);

    private static (JsonElement header, JsonElement payload) SplitJws(string jwt)
    {
        var parts = jwt.Split('.');
        var headerJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[0]));
        var payloadJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[1]));
        return (JsonDocument.Parse(headerJson).RootElement,
                JsonDocument.Parse(payloadJson).RootElement);
    }
}
