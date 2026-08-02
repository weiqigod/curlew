using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddPrCheckProviderDiscriminator : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<Guid>(
                name: "gitlab_installation_id",
                table: "pr_checks",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<long>(
                name: "gitlab_status_id",
                table: "pr_checks",
                type: "INTEGER",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "provider",
                table: "pr_checks",
                type: "TEXT",
                maxLength: 20,
                nullable: false,
                defaultValue: "github");

            migrationBuilder.CreateIndex(
                name: "idx_pr_checks_provider",
                table: "pr_checks",
                column: "provider");

            migrationBuilder.CreateIndex(
                name: "IX_pr_checks_gitlab_installation_id",
                table: "pr_checks",
                column: "gitlab_installation_id");

            migrationBuilder.AddCheckConstraint(
                name: "ck_pr_checks_provider",
                table: "pr_checks",
                sql: "\"provider\" IN ('github', 'gitlab')");

            migrationBuilder.AddForeignKey(
                name: "FK_pr_checks_gitlab_installations_gitlab_installation_id",
                table: "pr_checks",
                column: "gitlab_installation_id",
                principalTable: "gitlab_installations",
                principalColumn: "Id",
                onDelete: ReferentialAction.SetNull);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropForeignKey(
                name: "FK_pr_checks_gitlab_installations_gitlab_installation_id",
                table: "pr_checks");

            migrationBuilder.DropIndex(
                name: "idx_pr_checks_provider",
                table: "pr_checks");

            migrationBuilder.DropIndex(
                name: "IX_pr_checks_gitlab_installation_id",
                table: "pr_checks");

            migrationBuilder.DropCheckConstraint(
                name: "ck_pr_checks_provider",
                table: "pr_checks");

            migrationBuilder.DropColumn(
                name: "gitlab_installation_id",
                table: "pr_checks");

            migrationBuilder.DropColumn(
                name: "gitlab_status_id",
                table: "pr_checks");

            migrationBuilder.DropColumn(
                name: "provider",
                table: "pr_checks");
        }
    }
}
