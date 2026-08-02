using ApiTool.Backend.Data;

namespace ApiTool.Backend.Health;

/// <summary>DB health probe backed by Entity Framework's CanConnectAsync.</summary>
public sealed class EfDbHealthProbe(IServiceScopeFactory scopes) : IDbHealthProbe
{
    /// <inheritdoc/>
    public async Task<bool> IsConnectedAsync(CancellationToken ct)
    {
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(2));
        await using var scope = scopes.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        return await db.Database.CanConnectAsync(cts.Token);
    }
}
