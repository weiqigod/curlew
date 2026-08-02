using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddResultItemMethodAndPathTemplate : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.AddColumn<string>(
                name: "method",
                table: "result_items",
                type: "TEXT",
                maxLength: 10,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "path_template",
                table: "result_items",
                type: "TEXT",
                maxLength: 500,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "request_url",
                table: "result_items",
                type: "TEXT",
                maxLength: 1000,
                nullable: true);

            migrationBuilder.CreateIndex(
                name: "IX_result_items_Status_Method_PathTemplate",
                table: "result_items",
                columns: new[] { "Status", "method", "path_template" });
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "IX_result_items_Status_Method_PathTemplate",
                table: "result_items");

            migrationBuilder.DropColumn(
                name: "method",
                table: "result_items");

            migrationBuilder.DropColumn(
                name: "path_template",
                table: "result_items");

            migrationBuilder.DropColumn(
                name: "request_url",
                table: "result_items");
        }
    }
}
