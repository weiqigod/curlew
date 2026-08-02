using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddGithubWebhookEvents : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "github_webhook_events",
                columns: table => new
                {
                    delivery_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    event_type = table.Column<string>(type: "TEXT", maxLength: 50, nullable: false),
                    action = table.Column<string>(type: "TEXT", maxLength: 50, nullable: true),
                    received_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    processed_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    status = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    payload = table.Column<string>(type: "TEXT", nullable: false),
                    attempt_count = table.Column<int>(type: "INTEGER", nullable: false),
                    last_error = table.Column<string>(type: "TEXT", nullable: true),
                    last_error_at = table.Column<DateTime>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_github_webhook_events", x => x.delivery_id);
                });

            migrationBuilder.CreateIndex(
                name: "idx_github_webhook_events_status_received",
                table: "github_webhook_events",
                columns: new[] { "status", "received_at" });

            migrationBuilder.CreateIndex(
                name: "idx_github_webhook_events_type_received",
                table: "github_webhook_events",
                columns: new[] { "event_type", "received_at" });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "github_webhook_events");
        }
    }
}
