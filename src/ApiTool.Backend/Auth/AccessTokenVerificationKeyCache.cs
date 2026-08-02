using ApiTool.Backend.Licensing.Keys;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Caches the public ES256 keys used to validate CLI access tokens. A token with
/// an unknown <c>kid</c> bypasses the cache so newly rotated keys work immediately.
/// </summary>
public sealed class AccessTokenVerificationKeyCache(
    IServiceScopeFactory scopeFactory,
    TimeProvider clock)
{
    private static readonly TimeSpan CacheLifetime = TimeSpan.FromHours(1);
    private readonly SemaphoreSlim _refreshLock = new(1, 1);
    private Snapshot _snapshot = Snapshot.Empty;

    /// <summary>Returns the active and still-verifying public signing keys.</summary>
    public async Task<IReadOnlyList<SecurityKey>> GetAsync(string kid, CancellationToken ct)
    {
        var snapshot = _snapshot;
        if (snapshot.IsUsable(kid, clock.GetUtcNow()))
            return snapshot.Keys;

        await _refreshLock.WaitAsync(ct);
        try
        {
            snapshot = _snapshot;
            if (snapshot.IsUsable(kid, clock.GetUtcNow()))
                return snapshot.Keys;

            await using var scope = scopeFactory.CreateAsyncScope();
            var provider = scope.ServiceProvider.GetRequiredService<IKeyProvider>();
            var jwks = await provider.GetVerificationJwksAsync(ct);
            var keys = jwks.Keys.Cast<SecurityKey>().ToArray();
            _snapshot = new Snapshot(keys, clock.GetUtcNow().Add(CacheLifetime));
            return keys;
        }
        finally
        {
            _refreshLock.Release();
        }
    }

    private sealed record Snapshot(IReadOnlyList<SecurityKey> Keys, DateTimeOffset ExpiresAt)
    {
        public static readonly Snapshot Empty = new([], DateTimeOffset.MinValue);

        public bool IsUsable(string kid, DateTimeOffset now) =>
            ExpiresAt > now && Keys.Any(key => string.Equals(key.KeyId, kid, StringComparison.Ordinal));
    }
}
