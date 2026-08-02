// Refs docs/SPECIFICATION.md:8409-8427 (lifecycle), :10006-10025 (schema).
using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddGithubInstallations : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "github_installations",
                columns: table => new
                {
                    installation_id = table.Column<long>(type: "INTEGER", nullable: false),
                    app_id = table.Column<long>(type: "INTEGER", nullable: false),
                    org_id = table.Column<Guid>(type: "TEXT", nullable: true),
                    account_login = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    account_type = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    repo_selection = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    repo_set = table.Column<string>(type: "TEXT", nullable: false),
                    installed_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    claimed_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    suspended_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    deleted_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    last_reconciled_at = table.Column<DateTime>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_github_installations", x => x.installation_id);
                    table.ForeignKey(
                        name: "FK_github_installations_organizations_org_id",
                        column: x => x.org_id,
                        principalTable: "organizations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "idx_github_installations_alive",
                table: "github_installations",
                column: "installation_id",
                filter: "\"deleted_at\" IS NULL");

            migrationBuilder.CreateIndex(
                name: "idx_github_installations_app",
                table: "github_installations",
                column: "app_id");

            migrationBuilder.CreateIndex(
                name: "idx_github_installations_org",
                table: "github_installations",
                column: "org_id",
                unique: true,
                filter: "\"deleted_at\" IS NULL AND \"org_id\" IS NOT NULL");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "github_installations");
        }
    }
}
