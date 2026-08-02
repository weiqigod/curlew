using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <summary>
    /// Migration 2 of the M18-009 expand-contract pattern.
    /// Drops the plaintext <c>template_jsonb</c> column from <c>team_vaults</c> after the
    /// <see cref="ApiTool.Backend.VaultConfig.TeamVaultBackfillHost"/> has verified all rows
    /// are envelope-encrypted (ciphertext is NOT NULL for every row).
    /// <para>
    /// DEPLOYMENT SEQUENCING (must be followed to avoid data loss):
    /// <list type="number">
    /// <item>Deploy Migration 1 (<c>AddEncryptedTeamVaultAndScheduleEnvVars</c>) — adds new ciphertext columns.</item>
    /// <item>Restart the application — the <see cref="ApiTool.Backend.VaultConfig.TeamVaultBackfillHost"/>
    ///       runs at startup and encrypts every <c>template_jsonb</c> row.</item>
    /// <item>Verify backfill completion in application logs: look for "encryption-at-rest backfill: complete".</item>
    /// <item>Deploy Migration 2 (this migration) — the guard SQL below aborts if any unencrypted rows remain.</item>
    /// </list>
    /// </para>
    /// <para>
    /// ROLLBACK (Down path): Re-adds <c>template_jsonb</c> as a nullable TEXT column and copies the
    /// decrypted bytes back. Because the Down() method runs in a migration context without access to the
    /// <see cref="ApiTool.Backend.VaultConfig.Keys.ITeamVaultKeyProvider"/> DI service, the decryption
    /// is deferred: the Down migration re-adds the column and copies the ciphertext blob as a hex string
    /// into the plaintext column as a documented fallback. A post-rollback operator runbook step must
    /// then invoke the key provider out-of-band (e.g. a one-off script) to decrypt the blobs properly.
    /// This is documented in the CHANGELOG entry for M18-009.
    /// </para>
    /// Refs: M18-009 (v4-12), docs/SPECIFICATION.md:9196-9205 (expand-contract pattern).
    /// </summary>
    public partial class DropTeamVaultPlaintext : Migration
    {
        /// <inheritdoc/>
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            // Guard: abort if any row still has no ciphertext — backfill must complete first.
            // SQLite and PostgreSQL both support this syntax.
            migrationBuilder.Sql("""
                SELECT CASE
                    WHEN (SELECT COUNT(*) FROM team_vaults WHERE template_jsonb_ciphertext IS NULL) > 0
                    THEN RAISE(ABORT, 'DropTeamVaultPlaintext aborted: backfill incomplete — some rows still have NULL template_jsonb_ciphertext. Deploy Migration 1 and restart the application to complete the backfill before applying Migration 2.')
                END;
                """);

            migrationBuilder.DropColumn(
                name: "template_jsonb",
                table: "team_vaults");
        }

        /// <inheritdoc/>
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            // Re-add the plaintext column (nullable — rows will not have plaintext after this migration runs).
            // The column is populated with a hex encoding of the ciphertext blob as a documented placeholder.
            // An out-of-band decryption step is required to restore actual cleartext — see M18-009 CHANGELOG.
            migrationBuilder.AddColumn<string>(
                name: "template_jsonb",
                table: "team_vaults",
                type: "TEXT",
                nullable: true,
                defaultValue: null);

            // Copy ciphertext bytes as a hex string into the restored plaintext column.
            // This preserves recoverability without requiring DI-injected key provider in the migration.
            migrationBuilder.Sql("""
                UPDATE team_vaults
                SET template_jsonb = hex(template_jsonb_ciphertext)
                WHERE template_jsonb_ciphertext IS NOT NULL;
                """);
        }
    }
}
