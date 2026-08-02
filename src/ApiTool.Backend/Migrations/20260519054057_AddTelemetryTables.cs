using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddTelemetryTables : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "telemetry_daily_aggregates",
                columns: table => new
                {
                    day = table.Column<DateOnly>(type: "TEXT", nullable: false),
                    event_type = table.Column<string>(type: "TEXT", maxLength: 64, nullable: false),
                    event_count = table.Column<long>(type: "INTEGER", nullable: false),
                    distinct_install_count = table.Column<long>(type: "INTEGER", nullable: false),
                    numeric_sums = table.Column<string>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_telemetry_daily_aggregates", x => new { x.day, x.event_type });
                });

            migrationBuilder.CreateTable(
                name: "telemetry_events",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    install_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    event_type = table.Column<string>(type: "TEXT", maxLength: 64, nullable: false),
                    event_payload = table.Column<string>(type: "TEXT", nullable: false),
                    received_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    idempotency_key = table.Column<string>(type: "TEXT", maxLength: 64, nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_telemetry_events", x => x.Id);
                });

            migrationBuilder.CreateIndex(
                name: "idx_telemetry_events_idempotency_key",
                table: "telemetry_events",
                column: "idempotency_key",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "idx_telemetry_events_install_received",
                table: "telemetry_events",
                columns: new[] { "install_id", "received_at" });

            migrationBuilder.CreateIndex(
                name: "idx_telemetry_events_received_at",
                table: "telemetry_events",
                column: "received_at");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "telemetry_daily_aggregates");

            migrationBuilder.DropTable(
                name: "telemetry_events");
        }
    }
}
