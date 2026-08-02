// Refs docs/SPECIFICATION.md:8456-8470 (in-process installation token cache).
using System.Collections.Concurrent;
using System.Net.Http.Headers;
using System.Text.Json;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// In-process cache for GitHub App installation access tokens.
/// Uses single-flight refresh (one SemaphoreSlim per installation id) to avoid
/// thundering-herd token fetches under concurrent load.
/// Tokens are considered expired 5 minutes before their stated expiry to allow
/// for clock skew and in-flight request completion.
/// </summary>
public sealed class InstallationTokenCache(
    IHttpClientFactory httpFactory,
    IGitHubAppKeyProvider keyProvider,
    TimeProvider clock,
    ILogger<InstallationTokenCache> log) : IInstallationTokenCache
{
    private static readonly TimeSpan EarlyExpiry = TimeSpan.FromMinutes(5);

    // Per-installation cached token
    private readonly ConcurrentDictionary<long, CachedToken> _cache = new();

    // Per-installation semaphore for single-flight refresh
    private readonly ConcurrentDictionary<long, SemaphoreSlim> _locks = new();

    /// <inheritdoc/>
    public async Task<string> GetOrRefreshAsync(long installationId, CancellationToken ct)
    {
        var now = clock.GetUtcNow();

        // Fast path: valid cached token
        if (_cache.TryGetValue(installationId, out var cached) && !IsExpired(cached, now))
            return cached.Token;

        // Slow path: acquire per-installation lock for single-flight refresh
        var sem = _locks.GetOrAdd(installationId, _ => new SemaphoreSlim(1, 1));
        await sem.WaitAsync(ct).ConfigureAwait(false);
        try
        {
            // Re-check after acquiring the lock (another thread may have refreshed already)
            now = clock.GetUtcNow();
            if (_cache.TryGetValue(installationId, out cached) && !IsExpired(cached, now))
                return cached.Token;

            log.LogDebug("Fetching installation token for installation {InstallationId}", installationId);

            var token = await FetchTokenAsync(installationId, ct).ConfigureAwait(false);
            _cache[installationId] = token;
            return token.Token;
        }
        finally
        {
            sem.Release();
        }
    }

    /// <inheritdoc/>
    public void Evict(long installationId)
    {
        _cache.TryRemove(installationId, out _);
    }

    private bool IsExpired(CachedToken cached, DateTimeOffset now)
        => now >= cached.ExpiresAt - EarlyExpiry;

    private async Task<CachedToken> FetchTokenAsync(long installationId, CancellationToken ct)
    {
        var claims = new GitHubAppJwtClaims(
            AppId: keyProvider.GetAppId(),
            Iat: clock.GetUtcNow(),
            Exp: clock.GetUtcNow() + TimeSpan.FromMinutes(10));

        var jwt = await keyProvider.SignAppJwtAsync(claims, ct).ConfigureAwait(false);

        var http = httpFactory.CreateClient("github-app");
        using var req = new HttpRequestMessage(
            HttpMethod.Post,
            $"/app/installations/{installationId}/access_tokens");
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", jwt);
        req.Headers.Accept.ParseAdd("application/vnd.github+json");
        req.Headers.Add("X-GitHub-Api-Version", "2022-11-28");

        using var rsp = await http.SendAsync(req, ct).ConfigureAwait(false);
        rsp.EnsureSuccessStatusCode();

        var body = await rsp.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
        using var doc = JsonDocument.Parse(body);
        var root = doc.RootElement;

        var tokenValue = root.GetProperty("token").GetString()
            ?? throw new InvalidOperationException("GitHub access_tokens response missing 'token' field");
        var expiresAtStr = root.GetProperty("expires_at").GetString()
            ?? throw new InvalidOperationException("GitHub access_tokens response missing 'expires_at' field");
        var expiresAt = DateTimeOffset.Parse(expiresAtStr, null, System.Globalization.DateTimeStyles.RoundtripKind);

        log.LogDebug("Fetched installation token for installation {InstallationId}, expires {ExpiresAt}",
            installationId, expiresAt);

        return new CachedToken(tokenValue, expiresAt);
    }
}

/// <summary>A cached installation access token with its expiry time.</summary>
internal sealed record CachedToken(string Token, DateTimeOffset ExpiresAt);
