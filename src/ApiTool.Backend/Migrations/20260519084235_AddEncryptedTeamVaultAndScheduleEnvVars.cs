using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddEncryptedTeamVaultAndScheduleEnvVars : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<byte[]>(
                name: "template_jsonb_ciphertext",
                table: "team_vaults",
                type: "BLOB",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "template_jsonb_kid",
                table: "team_vaults",
                type: "TEXT",
                maxLength: 500,
                nullable: true);

            migrationBuilder.AddColumn<byte[]>(
                name: "env_vars_ciphertext",
                table: "schedules",
                type: "BLOB",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "env_vars_kid",
                table: "schedules",
                type: "TEXT",
                maxLength: 500,
                nullable: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropColumn(
                name: "template_jsonb_ciphertext",
                table: "team_vaults");

            migrationBuilder.DropColumn(
                name: "template_jsonb_kid",
                table: "team_vaults");

            migrationBuilder.DropColumn(
                name: "env_vars_ciphertext",
                table: "schedules");

            migrationBuilder.DropColumn(
                name: "env_vars_kid",
                table: "schedules");
        }
    }
}
