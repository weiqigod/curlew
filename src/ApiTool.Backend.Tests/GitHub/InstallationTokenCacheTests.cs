// Refs docs/SPECIFICATION.md:8456-8470 (in-process installation token cache).
using System.Net;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.GitHub;

/// <summary>
/// Unit tests for <see cref="InstallationTokenCache"/>.
/// Uses a FakeClock and FakeHttpMessageHandler for deterministic control.
/// </summary>
public sealed class InstallationTokenCacheTests
{
    private static readonly DateTimeOffset BaseTime = new(2026, 5, 7, 12, 0, 0, TimeSpan.Zero);

    private static FakeHttpMessageHandler BuildHandler(
        string token = "ghs_testtoken1234567890123456789012345678",
        string expiresAt = "2099-01-01T00:00:00Z")
    {
        var json = $$"""{"token":"{{token}}","expires_at":"{{expiresAt}}"}""";
        return new FakeHttpMessageHandler(HttpStatusCode.Created, json, "application/json");
    }

    private static InstallationTokenCache BuildCache(
        FakeHttpMessageHandler handler,
        FakeClock? clock = null,
        IGitHubAppKeyProvider? keyProvider = null)
    {
        var factory = new FakeHttpClientFactory(handler);
        var kp = keyProvider ?? new FakeGitHubAppKeyProvider("fake.jwt", appId: 12345L);
        var tp = clock ?? new FakeClock(BaseTime);
        return new InstallationTokenCache(factory, kp, tp, NullLogger<InstallationTokenCache>.Instance);
    }

    [Fact]
    public async Task FirstCall_FetchesViaJwt_CachesResult()
    {
        var handler = BuildHandler("ghs_cachedtoken1234567890123456789012345678");
        var cache = BuildCache(handler);

        var token = await cache.GetOrRefreshAsync(1001L, CancellationToken.None);

        token.Should().Be("ghs_cachedtoken1234567890123456789012345678");
        handler.RequestCount.Should().Be(1);
    }

    [Fact]
    public async Task SecondCall_SameInstallation_UsesCache_DoesNotRefetch()
    {
        var handler = BuildHandler("ghs_cachedtoken1234567890123456789012345678");
        var cache = BuildCache(handler);

        var t1 = await cache.GetOrRefreshAsync(1001L, CancellationToken.None);
        var t2 = await cache.GetOrRefreshAsync(1001L, CancellationToken.None);

        t1.Should().Be(t2);
        handler.RequestCount.Should().Be(1); // only one HTTP call
    }

    [Fact]
    public async Task TokenWithin5MinOfExpiry_TreatedAsExpired_RefetchedEarly()
    {
        var clock = new FakeClock(BaseTime);
        // Token expires in 4 minutes from "now" — within the 5-minute early-expiry window
        var expiresAt = (BaseTime + TimeSpan.FromMinutes(4)).ToString("O");
        var handler = BuildHandler("ghs_expiring1234567890123456789012345678", expiresAt);
        var cache = BuildCache(handler, clock);

        // First call — fetches and caches
        await cache.GetOrRefreshAsync(1001L, CancellationToken.None);
        handler.RequestCount.Should().Be(1);

        // Second call — token is within 5 min of expiry → should refetch
        await cache.GetOrRefreshAsync(1001L, CancellationToken.None);
        handler.RequestCount.Should().Be(2);
    }

    [Fact]
    public void Evict_RemovesEntry_DoesNotThrow()
    {
        var handler = BuildHandler("ghs_evicttest1234567890123456789012345678");
        var cache = BuildCache(handler);

        // Evicting a non-existent entry should not throw
        var act = () => cache.Evict(9999L);
        act.Should().NotThrow();
    }
}
