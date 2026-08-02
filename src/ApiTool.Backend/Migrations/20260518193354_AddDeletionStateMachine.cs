using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddDeletionStateMachine : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<DateTime>(
                name: "anonymised_at",
                table: "users",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<DateTime>(
                name: "pending_deletion_at",
                table: "users",
                type: "TEXT",
                nullable: true);

            migrationBuilder.CreateTable(
                name: "deletion_reauth_tokens",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    user_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    token_hash = table.Column<byte[]>(type: "BLOB", nullable: false),
                    issued_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    expires_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    consumed_at = table.Column<DateTime>(type: "TEXT", nullable: true)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_deletion_reauth_tokens", x => x.Id);
                    table.ForeignKey(
                        name: "FK_deletion_reauth_tokens_users_user_id",
                        column: x => x.user_id,
                        principalTable: "users",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "idx_deletion_reauth_tokens_active",
                table: "deletion_reauth_tokens",
                column: "expires_at",
                filter: "\"consumed_at\" IS NULL");

            migrationBuilder.CreateIndex(
                name: "idx_deletion_reauth_tokens_hash",
                table: "deletion_reauth_tokens",
                column: "token_hash",
                unique: true);

            migrationBuilder.CreateIndex(
                name: "idx_deletion_reauth_tokens_user",
                table: "deletion_reauth_tokens",
                columns: new[] { "user_id", "issued_at" },
                descending: new[] { false, true });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "deletion_reauth_tokens");

            migrationBuilder.DropColumn(
                name: "anonymised_at",
                table: "users");

            migrationBuilder.DropColumn(
                name: "pending_deletion_at",
                table: "users");
        }
    }
}
