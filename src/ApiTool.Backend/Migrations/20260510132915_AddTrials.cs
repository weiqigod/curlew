using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddTrials : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "trials",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    user_id = table.Column<Guid>(type: "TEXT", nullable: false),
                    feature = table.Column<string>(type: "TEXT", maxLength: 64, nullable: false),
                    kind = table.Column<string>(type: "TEXT", maxLength: 32, nullable: false),
                    granted_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    expires_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    consumed_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    notified_3day_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    notified_1day_at = table.Column<DateTime>(type: "TEXT", nullable: true),
                    created_at = table.Column<DateTime>(type: "TEXT", nullable: false),
                    updated_at = table.Column<DateTime>(type: "TEXT", nullable: false)
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_trials", x => x.Id);
                    table.CheckConstraint("ck_trials_kind", "\"kind\" IN ('full_initial', 'ondemand', 'preempted_by_subscription')");
                    table.ForeignKey(
                        name: "FK_trials_users_user_id",
                        column: x => x.user_id,
                        principalTable: "users",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "idx_trials_notify_1day",
                table: "trials",
                column: "expires_at",
                filter: "\"notified_1day_at\" IS NULL AND \"consumed_at\" IS NULL");

            // idx_trials_notify_3day: partial index for the 3-day expiry-warning cron (M16-008).
            // Created via raw SQL because EF Core's model tracks only one HasIndex per property;
            // this index is idempotent with Down() (DropTable cascades all indexes).
            migrationBuilder.Sql(
                """
                CREATE INDEX IF NOT EXISTS "idx_trials_notify_3day"
                ON "trials" ("expires_at")
                WHERE "notified_3day_at" IS NULL AND "consumed_at" IS NULL;
                """);

            migrationBuilder.CreateIndex(
                name: "idx_trials_user",
                table: "trials",
                column: "user_id");

            migrationBuilder.CreateIndex(
                name: "idx_trials_user_feature",
                table: "trials",
                columns: new[] { "user_id", "feature" },
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(
                name: "trials");
        }
    }
}
