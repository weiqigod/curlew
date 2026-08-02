using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AnonymiseSetNullColumns : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_coordinator_jobs_users_CreatedBy",
                table: "coordinator_jobs");

            migrationBuilder.DropForeignKey(
                name: "FK_notification_rules_users_CreatedBy",
                table: "notification_rules");

            migrationBuilder.DropForeignKey(
                name: "FK_organization_custom_roles_users_CreatedBy",
                table: "organization_custom_roles");

            migrationBuilder.DropForeignKey(
                name: "FK_schedules_users_CreatedBy",
                table: "schedules");

            migrationBuilder.DropForeignKey(
                name: "FK_team_vaults_users_created_by",
                table: "team_vaults");

            migrationBuilder.DropForeignKey(
                name: "FK_team_vaults_users_updated_by",
                table: "team_vaults");

            migrationBuilder.RenameColumn(
                name: "ActorId",
                table: "organization_audit_log",
                newName: "actor_id");

            migrationBuilder.AlterColumn<Guid>(
                name: "updated_by",
                table: "team_vaults",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "created_by",
                table: "team_vaults",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "schedules",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "organization_custom_roles",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "actor_id",
                table: "organization_audit_log",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "notification_rules",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "coordinator_jobs",
                type: "TEXT",
                nullable: true,
                oldClrType: typeof(Guid),
                oldType: "TEXT");

            migrationBuilder.AddForeignKey(
                name: "FK_coordinator_jobs_users_CreatedBy",
                table: "coordinator_jobs",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);

            migrationBuilder.AddForeignKey(
                name: "FK_notification_rules_users_CreatedBy",
                table: "notification_rules",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);

            migrationBuilder.AddForeignKey(
                name: "FK_organization_custom_roles_users_CreatedBy",
                table: "organization_custom_roles",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);

            migrationBuilder.AddForeignKey(
                name: "FK_schedules_users_CreatedBy",
                table: "schedules",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);

            migrationBuilder.AddForeignKey(
                name: "FK_team_vaults_users_created_by",
                table: "team_vaults",
                column: "created_by",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);

            migrationBuilder.AddForeignKey(
                name: "FK_team_vaults_users_updated_by",
                table: "team_vaults",
                column: "updated_by",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_coordinator_jobs_users_CreatedBy",
                table: "coordinator_jobs");

            migrationBuilder.DropForeignKey(
                name: "FK_notification_rules_users_CreatedBy",
                table: "notification_rules");

            migrationBuilder.DropForeignKey(
                name: "FK_organization_custom_roles_users_CreatedBy",
                table: "organization_custom_roles");

            migrationBuilder.DropForeignKey(
                name: "FK_schedules_users_CreatedBy",
                table: "schedules");

            migrationBuilder.DropForeignKey(
                name: "FK_team_vaults_users_created_by",
                table: "team_vaults");

            migrationBuilder.DropForeignKey(
                name: "FK_team_vaults_users_updated_by",
                table: "team_vaults");

            migrationBuilder.RenameColumn(
                name: "actor_id",
                table: "organization_audit_log",
                newName: "ActorId");

            migrationBuilder.AlterColumn<Guid>(
                name: "updated_by",
                table: "team_vaults",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "created_by",
                table: "team_vaults",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "schedules",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "organization_custom_roles",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "ActorId",
                table: "organization_audit_log",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "notification_rules",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AlterColumn<Guid>(
                name: "CreatedBy",
                table: "coordinator_jobs",
                type: "TEXT",
                nullable: false,
                defaultValue: new Guid("00000000-0000-0000-0000-000000000000"),
                oldClrType: typeof(Guid),
                oldType: "TEXT",
                oldNullable: true);

            migrationBuilder.AddForeignKey(
                name: "FK_coordinator_jobs_users_CreatedBy",
                table: "coordinator_jobs",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);

            migrationBuilder.AddForeignKey(
                name: "FK_notification_rules_users_CreatedBy",
                table: "notification_rules",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);

            migrationBuilder.AddForeignKey(
                name: "FK_organization_custom_roles_users_CreatedBy",
                table: "organization_custom_roles",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);

            migrationBuilder.AddForeignKey(
                name: "FK_schedules_users_CreatedBy",
                table: "schedules",
                column: "CreatedBy",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);

            migrationBuilder.AddForeignKey(
                name: "FK_team_vaults_users_created_by",
                table: "team_vaults",
                column: "created_by",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);

            migrationBuilder.AddForeignKey(
                name: "FK_team_vaults_users_updated_by",
                table: "team_vaults",
                column: "updated_by",
                principalTable: "users",
                principalColumn: "Id",
                onDelete: ReferentialAction.Restrict);
        }
    }
}
