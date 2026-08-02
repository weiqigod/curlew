using ApiTool.Backend.Data;
using ApiTool.Backend.VaultConfig.Keys;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.VaultConfig;

/// <summary>
/// One-shot startup background service that encrypts any <c>team_vaults</c> rows that still have
/// a plaintext <c>template_json</c> but no <c>template_jsonb_ciphertext</c>.
/// <para>
/// This runs after EF migrations so the new columns exist.  Once backfill completes, a separate
/// Migration 2 (<c>DropTeamVaultPlaintext</c>) can safely drop the plaintext column.
/// </para>
/// <para>
/// Edge case — empty table (fresh install): no rows → no-op. Already-encrypted rows (ciphertext
/// not null) are skipped to make the host idempotent on repeated restarts.
/// </para>
/// Refs: M18-009 (v4-12), docs/SPECIFICATION.md:9196-9205 (expand-contract pattern).
/// </summary>
internal sealed class TeamVaultBackfillHost(
    IServiceProvider sp,
    ITeamVaultKeyProvider keyProvider,
    ILogger<TeamVaultBackfillHost> logger) : IHostedService
{
    private const int BatchSize = 100;

    /// <inheritdoc/>
    public async Task StartAsync(CancellationToken cancellationToken)
    {
        using var scope = sp.CreateScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();

        int pendingCount;
        try
        {
            pendingCount = await db.TeamVaults
                .CountAsync(v => v.TemplateJsonCiphertext == null, cancellationToken);
        }
        catch (Exception ex) when (ex.Message.Contains("no such table") || ex.Message.Contains("relation") || ex.Message.Contains("does not exist"))
        {
            logger.LogWarning("encryption-at-rest backfill: team_vaults table not found — skipping (migrations not yet applied?)");
            return;
        }

        if (pendingCount == 0)
        {
            logger.LogInformation("encryption-at-rest backfill: already complete (no unencrypted team_vaults rows)");
            return;
        }

        logger.LogInformation(
            "encryption-at-rest backfill: {Count} team_vaults row(s) need encryption — starting",
            pendingCount);

        var processed = 0;
        while (true)
        {
            var batch = await db.TeamVaults
                .Where(v => v.TemplateJsonCiphertext == null && v.TemplateJson != null)
                .OrderBy(v => v.OrgId)
                .Take(BatchSize)
                .ToListAsync(cancellationToken);

            if (batch.Count == 0)
                break;

            foreach (var vault in batch)
            {
                var plainBytes = System.Text.Encoding.UTF8.GetBytes(vault.TemplateJson);
                var result = await keyProvider.EncryptAsync(plainBytes, cancellationToken);
                vault.TemplateJsonCiphertext = result.Ciphertext;
                vault.TemplateJsonKid = result.Kid;
            }

            await db.SaveChangesAsync(cancellationToken);
            processed += batch.Count;
            logger.LogInformation("encryption-at-rest backfill: {Processed}/{Total} rows done", processed, pendingCount);
        }

        // Verification — after backfill no row should have null ciphertext.
        var remaining = await db.TeamVaults
            .CountAsync(v => v.TemplateJsonCiphertext == null, cancellationToken);

        if (remaining > 0)
        {
            logger.LogError(
                "encryption-at-rest backfill: verification FAILED — {Remaining} row(s) still unencrypted. " +
                "Refusing to start with partially-encrypted data.",
                remaining);
            throw new InvalidOperationException(
                $"TeamVault backfill verification failed: {remaining} row(s) remain unencrypted.");
        }

        logger.LogInformation("encryption-at-rest backfill: complete — {Processed} row(s) encrypted", processed);
    }

    /// <inheritdoc/>
    public Task StopAsync(CancellationToken cancellationToken) => Task.CompletedTask;
}
