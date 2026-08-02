using System.Text;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Tokens;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Licensing.Tokens;

public sealed class LicenseTokenIssuerTrialIntegrationTests : IAsyncDisposable
{
    private readonly string _tmpDir;

    public LicenseTokenIssuerTrialIntegrationTests()
    {
        _tmpDir = Path.Combine(Path.GetTempPath(), $"apitool_lic_trial_test_{Guid.NewGuid():N}");
        Directory.CreateDirectory(_tmpDir);
    }

    public async ValueTask DisposeAsync()
    {
        if (Directory.Exists(_tmpDir))
            Directory.Delete(_tmpDir, recursive: true);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task IssueAsync_populates_trial_state_active_with_expiry_when_resolver_returns_active()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);

        var unixExpiry = DateTimeOffset.UtcNow.AddDays(7).ToUnixTimeSeconds();
        var resolver = new FakeTrialStateResolver
        {
            Result = new TrialStateResult("active", unixExpiry, ["schedules"]),
        };
        var sut = CreateIssuer(keyProvider, resolver);

        var jwt = await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "free",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: Array.Empty<string>(),
            RequestLimit: 1000));

        var (_, payload) = SplitJws(jwt);
        payload.GetProperty("trial_state").GetString().Should().Be("active");
        payload.GetProperty("trial_expiry").GetInt64().Should().Be(unixExpiry);
    }

    [Fact]
    public async Task IssueAsync_unions_trialing_features_into_features_claim()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);

        // Resolver returns trialing features including one that overlaps with input.
        var resolver = new FakeTrialStateResolver
        {
            Result = new TrialStateResult("active", 1L, ["schedules", "vault_provider_profiles"]),
        };
        var sut = CreateIssuer(keyProvider, resolver);

        var jwt = await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "free",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: ["vault_provider_profiles"],
            RequestLimit: 1000));

        var (_, payload) = SplitJws(jwt);
        var features = payload.GetProperty("features").EnumerateArray()
            .Select(e => e.GetString()!)
            .ToList();
        features.Should().Contain("schedules").And.Contain("vault_provider_profiles");
        features.Should().OnlyHaveUniqueItems("trialing features union must deduplicate");
    }

    [Fact]
    public async Task IssueAsync_passes_input_tier_to_resolver()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();
        var (keyProvider, _) = CreateKeyProvider(scope.Db);

        var resolver = new FakeTrialStateResolver();
        var sut = CreateIssuer(keyProvider, resolver);

        await sut.IssueAsync(new LicenseTokenInput(
            UserId: Guid.NewGuid(),
            Email: "user@example.com",
            Tier: "professional",
            OrgId: null,
            OrgRole: null,
            DeviceId: Guid.NewGuid(),
            Features: Array.Empty<string>(),
            RequestLimit: 1000));

        resolver.LastTier.Should().Be("professional",
            because: "issuer must pass the input tier to the resolver");
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

    private static LicenseTokenIssuer CreateIssuer(IKeyProvider keyProvider, ITrialStateResolver? resolver = null) =>
        new(keyProvider, Options.Create(new TokenIssuerOptions()), TimeProvider.System,
            resolver ?? new FakeTrialStateResolver());

    private static (JsonElement header, JsonElement payload) SplitJws(string jwt)
    {
        var parts = jwt.Split('.');
        var headerJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[0]));
        var payloadJson = Encoding.UTF8.GetString(
            Microsoft.IdentityModel.Tokens.Base64UrlEncoder.DecodeBytes(parts[1]));
        return (JsonDocument.Parse(headerJson).RootElement, JsonDocument.Parse(payloadJson).RootElement);
    }
}
