using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddGitlabInstallationsAndEvents : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "gitlab_installations",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    org_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    project_id = table.Column<long>(type: "INTEGER", nullable: false),
                    project_path = table.Column<string>(type: "TEXT", maxLength: 500, nullable: false),
                    gitlab_base_url = table.Column<string>(type: "TEXT", maxLength: 500, nullable: false),
                    gitlab_ca_bundle = table.Column<string>(type: "TEXT", nullable: true),
                    access_token_ciphertext = table.Column<byte[]>(type: "BLOB", nullable: false),
                    access_token_kid = table.Column<string>(type: "TEXT", maxLength: 500, nullable: false),
                    access_token_revoked_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    webhook_secret_ciphertext = table.Column<byte[]>(type: "BLOB", nullable: true),
                    webhook_secret_rotated_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    created_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    updated_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    deleted_at = table.Column<DateTime>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_gitlab_installations", x => x.Id);
                    table.ForeignKey(
                        name: "FK_gitlab_installations_organizations_org_id",
                        column: x => x.org_id,
                        principalTable: "organizations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateTable(
                name: "gitlab_webhook_events",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    event_uuid = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    event_type = table.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                    installation_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    received_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    processed_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    failure_count = table.Column<int>(type: "INTEGER", nullable: false, defaultValue: 0),
                    quarantined_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    payload = table.Column<string>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_gitlab_webhook_events", x => x.Id);
                    table.ForeignKey(
                        name: "FK_gitlab_webhook_events_gitlab_installations_installation_id",
                        column: x => x.installation_id,
                        principalTable: "gitlab_installations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "idx_gitlab_installations_org_project_base",
                table: "gitlab_installations",
                columns: new[] { "org_id", "project_id", "gitlab_base_url" },
                unique: true,
                filter: "\"deleted_at\" IS NULL");

            migrationBuilder.CreateIndex(
                name: "idx_gitlab_webhook_events_uuid",
                table: "gitlab_webhook_events",
                column: "event_uuid",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_gitlab_webhook_events_installation_id",
                table: "gitlab_webhook_events",
                column: "installation_id");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "gitlab_webhook_events");

            migrationBuilder.DropTable(
                name: "gitlab_installations");
        }
    }
}
