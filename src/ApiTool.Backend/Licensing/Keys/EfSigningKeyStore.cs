using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// EF Core–backed implementation of <see cref="ISigningKeyStore"/>.
/// </summary>
public sealed class EfSigningKeyStore(AppDbContext db) : ISigningKeyStore
{
    /// <inheritdoc/>
    public async Task InsertAsync(SigningKey row, CancellationToken ct = default)
    {
        db.SigningKeys.Add(row);
        await db.SaveChangesAsync(ct);
    }

    /// <inheritdoc/>
    public async Task<SigningKey?> LoadByKidAsync(string kid, CancellationToken ct = default)
        => await db.SigningKeys.AsNoTracking()
            .FirstOrDefaultAsync(k => k.Kid == kid, ct);

    /// <inheritdoc/>
    public async Task<SigningKey?> LoadCurrentAsync(CancellationToken ct = default)
        => await db.SigningKeys.AsNoTracking()
            .FirstOrDefaultAsync(k => k.Status == KeyStatus.Current, ct);

    /// <inheritdoc/>
    public async Task<SigningKey?> LoadNextAsync(CancellationToken ct = default)
        => await db.SigningKeys.AsNoTracking()
            .FirstOrDefaultAsync(k => k.Status == KeyStatus.Next, ct);

    /// <inheritdoc/>
    public async Task<IReadOnlyList<SigningKey>> LoadCurrentAndVerifyingAsync(CancellationToken ct = default)
        => await db.SigningKeys.AsNoTracking()
            .Where(k => k.Status == KeyStatus.Current || k.Status == KeyStatus.Verifying)
            .ToListAsync(ct);

    /// <inheritdoc/>
    public async Task<bool> UpdateStatusAsync(
        string kid,
        string newStatus,
        string? revokeReason,
        TimeProvider clock,
        CancellationToken ct = default)
    {
        var row = await db.SigningKeys.FirstOrDefaultAsync(k => k.Kid == kid, ct);
        if (row is null)
            return false;

        row.Status = newStatus;
        if (newStatus == KeyStatus.Revoked)
        {
            row.RevokedAt = clock.GetUtcNow().UtcDateTime;
            row.RevokeReason = revokeReason;
        }
        if (newStatus == KeyStatus.Current)
        {
            row.PromotedAt = clock.GetUtcNow().UtcDateTime;
        }

        await db.SaveChangesAsync(ct);
        return true;
    }
}
