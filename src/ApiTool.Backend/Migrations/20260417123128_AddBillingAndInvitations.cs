using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddBillingAndInvitations : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_organization_invitations_OrgId_Email",
                table: "organization_invitations");

            migrationBuilder.DropIndex(
                name: "IX_organization_invitations_Token",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "Token",
                table: "organization_invitations");

            migrationBuilder.AddColumn<string>(
                name: "EmailNormalized",
                table: "organization_invitations",
                type: "TEXT",
                maxLength: 255,
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<DateTime>(
                name: "LastSentAt",
                table: "organization_invitations",
                type: "TEXT",
                nullable: false,
                defaultValue: new DateTime(1, 1, 1, 0, 0, 0, 0, DateTimeKind.Unspecified));

            migrationBuilder.AddColumn<DateTime>(
                name: "RevokedAt",
                table: "organization_invitations",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<Guid>(
                name: "RevokedBy",
                table: "organization_invitations",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "SendCount",
                table: "organization_invitations",
                type: "INTEGER",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<string>(
                name: "token_hash",
                table: "organization_invitations",
                type: "TEXT",
                maxLength: 64,
                nullable: false,
                defaultValue: "");

            migrationBuilder.CreateTable(
                name: "subscriptions",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    OrgId = table.Column<Guid>(type: "TEXT", nullable: false),
                    Tier = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    Status = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    Interval = table.Column<string>(type: "TEXT", maxLength: 10, nullable: false),
                    SeatCount = table.Column<int>(type: "INTEGER", nullable: false),
                    SeatLimit = table.Column<int>(type: "INTEGER", nullable: false),
                    CurrentPeriodStart = table.Column<DateTime>(type: "TEXT", nullable: false),
                    CurrentPeriodEnd = table.Column<DateTime>(type: "TEXT", nullable: false),
                    CancelAtPeriodEnd = table.Column<bool>(type: "INTEGER", nullable: false),
                    StripeCustomerId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: true),
                    StripeSubscriptionId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: true),
                    CreatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    UpdatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    CanceledAt = table.Column<DateTime>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_subscriptions", x => x.Id);
                    table.ForeignKey(
                        name: "FK_subscriptions_organizations_OrgId",
                        column: x => x.OrgId,
                        principalTable: "organizations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_organization_invitations_OrgId_EmailNormalized_AcceptedAt_RevokedAt",
                table: "organization_invitations",
                columns: new[] { "OrgId", "EmailNormalized", "AcceptedAt", "RevokedAt" });

            migrationBuilder.CreateIndex(
                name: "IX_organization_invitations_token_hash",
                table: "organization_invitations",
                column: "token_hash",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "IX_subscriptions_OrgId",
                table: "subscriptions",
                column: "OrgId",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "subscriptions");

            migrationBuilder.DropIndex(
                name: "IX_organization_invitations_OrgId_EmailNormalized_AcceptedAt_RevokedAt",
                table: "organization_invitations");

            migrationBuilder.DropIndex(
                name: "IX_organization_invitations_token_hash",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "EmailNormalized",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "LastSentAt",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "RevokedAt",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "RevokedBy",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "SendCount",
                table: "organization_invitations");

            migrationBuilder.DropColumn(
                name: "token_hash",
                table: "organization_invitations");

            migrationBuilder.AddColumn<string>(
                name: "Token",
                table: "organization_invitations",
                type: "TEXT",
                maxLength: 512,
                nullable: false,
                defaultValue: "");

            migrationBuilder.CreateIndex(
                name: "IX_organization_invitations_OrgId_Email",
                table: "organization_invitations",
                columns: new[] { "OrgId", "Email" });

            migrationBuilder.CreateIndex(
                name: "IX_organization_invitations_Token",
                table: "organization_invitations",
                column: "Token",
                unique: true);
        }
    }
}
