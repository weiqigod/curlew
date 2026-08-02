// Refs docs/SPECIFICATION.md:6801–6806 (invoice.* and payment_method.* event handlers).
using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class AddInvoicesAndPaymentMethods : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.CreateTable(
                name: "invoices",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    OrgId = table.Column<Guid>(type: "TEXT", nullable: false),
                    StripeInvoiceId = table.Column<string>(type: "TEXT", maxLength: 255, nullable: false),
                    StripeCustomerId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                    StripeSubscriptionId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: true),
                    Status = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    AmountPaid = table.Column<long>(type: "INTEGER", nullable: false),
                    AmountTotal = table.Column<long>(type: "INTEGER", nullable: false),
                    Currency = table.Column<string>(type: "TEXT", maxLength: 3, nullable: false),
                    HostedInvoiceUrl = table.Column<string>(type: "TEXT", maxLength: 500, nullable: true),
                    PeriodStart = table.Column<DateTime>(type: "TEXT", nullable: false),
                    PeriodEnd = table.Column<DateTime>(type: "TEXT", nullable: false),
                    AttemptCount = table.Column<int>(type: "INTEGER", nullable: false),
                    CreatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    UpdatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_invoices", x => x.Id);
                    table.ForeignKey(
                        name: "FK_invoices_organizations_OrgId",
                        column: x => x.OrgId,
                        principalTable: "organizations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_invoices_OrgId_CreatedAt",
                table: "invoices",
                columns: new[] { "OrgId", "CreatedAt" });

            migrationBuilder.CreateIndex(
                name: "IX_invoices_StripeInvoiceId",
                table: "invoices",
                column: "StripeInvoiceId",
                unique: true);

            migrationBuilder.CreateTable(
                name: "payment_methods",
                columns: table => new
                {
                    Id = table.Column<Guid>(type: "TEXT", nullable: false),
                    OrgId = table.Column<Guid>(type: "TEXT", nullable: false),
                    StripePaymentMethodId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                    StripeCustomerId = table.Column<string>(type: "TEXT", maxLength: 100, nullable: false),
                    Brand = table.Column<string>(type: "TEXT", maxLength: 20, nullable: false),
                    Last4 = table.Column<string>(type: "TEXT", maxLength: 4, nullable: false),
                    ExpMonth = table.Column<long>(type: "INTEGER", nullable: false),
                    ExpYear = table.Column<long>(type: "INTEGER", nullable: false),
                    AttachedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    DetachedAt = table.Column<DateTime>(type: "TEXT", nullable: true),
                    CreatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                    UpdatedAt = table.Column<DateTime>(type: "TEXT", nullable: false),
                },
                constraints: table =>
                {
                    table.PrimaryKey("PK_payment_methods", x => x.Id);
                    table.ForeignKey(
                        name: "FK_payment_methods_organizations_OrgId",
                        column: x => x.OrgId,
                        principalTable: "organizations",
                        principalColumn: "Id",
                        onDelete: ReferentialAction.Cascade);
                });

            migrationBuilder.CreateIndex(
                name: "IX_payment_methods_OrgId_AttachedAt",
                table: "payment_methods",
                columns: new[] { "OrgId", "AttachedAt" });

            migrationBuilder.CreateIndex(
                name: "IX_payment_methods_StripePaymentMethodId",
                table: "payment_methods",
                column: "StripePaymentMethodId",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropTable(name: "payment_methods");
            migrationBuilder.DropTable(name: "invoices");
        }
    }
}
