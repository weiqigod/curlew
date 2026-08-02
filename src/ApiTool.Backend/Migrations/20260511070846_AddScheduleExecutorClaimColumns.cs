using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddScheduleExecutorClaimColumns : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<DateTime>(
                name: "ClaimDeadline",
                table: "scheduled_runs",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "ClaimToken",
                table: "scheduled_runs",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<DateTime>(
                name: "ClaimedAt",
                table: "scheduled_runs",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "ClaimedByWorker",
                table: "scheduled_runs",
                type: "TEXT",
                maxLength: 200,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "FailureReason",
                table: "scheduled_runs",
                type: "TEXT",
                maxLength: 2000,
                nullable: true);

            migrationBuilder.AddColumn<DateTime>(
                name: "LastHeartbeatAt",
                table: "scheduled_runs",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "ResultId",
                table: "scheduled_runs",
                type: "TEXT",
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_scheduled_runs_LastHeartbeatAt",
                table: "scheduled_runs",
                column: "LastHeartbeatAt");

            migrationBuilder.CreateIndex(
                name: "IX_scheduled_runs_ResultId",
                table: "scheduled_runs",
                column: "ResultId");

            migrationBuilder.CreateIndex(
                name: "IX_scheduled_runs_Status_CreatedAt",
                table: "scheduled_runs",
                columns: new[] { "Status", "CreatedAt" });

            migrationBuilder.AddForeignKey(
                name: "FK_scheduled_runs_results_ResultId",
                table: "scheduled_runs",
                column: "ResultId",
                principalTable: "results",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_scheduled_runs_results_ResultId",
                table: "scheduled_runs");

            migrationBuilder.DropIndex(
                name: "IX_scheduled_runs_LastHeartbeatAt",
                table: "scheduled_runs");

            migrationBuilder.DropIndex(
                name: "IX_scheduled_runs_ResultId",
                table: "scheduled_runs");

            migrationBuilder.DropIndex(
                name: "IX_scheduled_runs_Status_CreatedAt",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "ClaimDeadline",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "ClaimToken",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "ClaimedAt",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "ClaimedByWorker",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "FailureReason",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "LastHeartbeatAt",
                table: "scheduled_runs");

            migrationBuilder.DropColumn(
                name: "ResultId",
                table: "scheduled_runs");
        }
    }
}
