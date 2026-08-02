using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddAuditLogDetails : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "failure_reason",
                table: "organization_audit_log",
                type: "TEXT",
                maxLength: 200,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "ip_address",
                table: "organization_audit_log",
                type: "TEXT",
                maxLength: 45,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "new_state",
                table: "organization_audit_log",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "previous_state",
                table: "organization_audit_log",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<bool>(
                name: "success",
                table: "organization_audit_log",
                type: "INTEGER",
                nullable: false,
                defaultValue: true);

            migrationBuilder.AddColumn<string>(
                name: "target_type",
                table: "organization_audit_log",
                type: "TEXT",
                maxLength: 50,
                nullable: true);

            migrationBuilder.AddColumn<System.Guid>(
                name: "target_id",
                table: "organization_audit_log",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "user_agent",
                table: "organization_audit_log",
                type: "TEXT",
                maxLength: 500,
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_organization_audit_log_EventType",
                table: "organization_audit_log",
                column: "EventType");
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_organization_audit_log_EventType",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "failure_reason",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "ip_address",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "new_state",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "previous_state",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "success",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "target_type",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "target_id",
                table: "organization_audit_log");

            migrationBuilder.DropColumn(
                name: "user_agent",
                table: "organization_audit_log");
        }
    }
}
