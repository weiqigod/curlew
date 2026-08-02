// Refs docs/SPECIFICATION.md:8429-8550 (pr_checks v4.2.1 schema expansion).
using System;
using Microsoft.EntityFrameworkCore.Migrations;

#nullable disable

namespace ApiTool.Backend.Migrations
{
    /// <inheritdoc />
    public partial class PrChecksV421 : Migration
    {
        /// <inheritdoc />
        protected override void Up(MigrationBuilder migrationBuilder)
        {
            // Additive columns only — no drops, no renames, no constraint changes on existing columns.

            migrationBuilder.AddColumn<long>(
                name: "installation_id",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "bigint" : "INTEGER",
                nullable: true);

            migrationBuilder.AddColumn<long>(
                name: "check_run_id",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "bigint" : "INTEGER",
                nullable: true);

            // external_id: NOT NULL UUID; app code always sets it explicitly on INSERT.
            // A zero-UUID default is safe for backfill because no existing rows have meaningful
            // external_id values (the column is new). Postgres uses gen_random_uuid() for proper backfill.
            if (migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL")
            {
                migrationBuilder.AddColumn<Guid>(
                    name: "external_id",
                    table: "pr_checks",
                    type: "uuid",
                    nullable: false,
                    defaultValueSql: "gen_random_uuid()");
            }
            else
            {
                // SQLite: constant empty-UUID default (safe — column is new, backfill is a no-op).
                migrationBuilder.AddColumn<Guid>(
                    name: "external_id",
                    table: "pr_checks",
                    type: "TEXT",
                    nullable: false,
                    defaultValue: Guid.Empty);
            }

            migrationBuilder.AddColumn<string>(
                name: "conclusion",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "character varying(20)" : "TEXT",
                maxLength: 20,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "details_url",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "character varying(500)" : "TEXT",
                maxLength: 500,
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "output_summary",
                table: "pr_checks",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "output_text",
                table: "pr_checks",
                type: "TEXT",
                nullable: true);

            // annotations: JSONB on Postgres for indexable JSON; plain TEXT on SQLite.
            migrationBuilder.AddColumn<string>(
                name: "annotations",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "jsonb" : "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<string>(
                name: "head_sha",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "character varying(64)" : "TEXT",
                maxLength: 64,
                nullable: false,
                defaultValue: "");

            migrationBuilder.AddColumn<DateTime>(
                name: "posting_started_at",
                table: "pr_checks",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<DateTime>(
                name: "posted_at",
                table: "pr_checks",
                type: "TEXT",
                nullable: true);

            migrationBuilder.AddColumn<int>(
                name: "attempt_count",
                table: "pr_checks",
                type: "INTEGER",
                nullable: false,
                defaultValue: 0);

            migrationBuilder.AddColumn<string>(
                name: "last_error",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "character varying(500)" : "TEXT",
                maxLength: 500,
                nullable: true);

            // status: backfill from existing state column.
            migrationBuilder.AddColumn<string>(
                name: "status",
                table: "pr_checks",
                type: migrationBuilder.ActiveProvider == "Npgsql.EntityFrameworkCore.PostgreSQL" ? "character varying(30)" : "TEXT",
                maxLength: 30,
                nullable: false,
                defaultValue: "pending");

            // Backfill status based on existing state value.
            migrationBuilder.Sql(@"
UPDATE pr_checks SET status = CASE
    WHEN state = 'success' THEN 'posted'
    WHEN state = 'failure' THEN 'posted'
    ELSE 'pending' END;");

            // Unique index on external_id for idempotency-on-retry.
            migrationBuilder.CreateIndex(
                name: "idx_pr_checks_external_id",
                table: "pr_checks",
                column: "external_id",
                unique: true);
        }

        /// <inheritdoc />
        protected override void Down(MigrationBuilder migrationBuilder)
        {
            migrationBuilder.DropIndex(
                name: "idx_pr_checks_external_id",
                table: "pr_checks");

            migrationBuilder.DropColumn(name: "status",             table: "pr_checks");
            migrationBuilder.DropColumn(name: "last_error",         table: "pr_checks");
            migrationBuilder.DropColumn(name: "attempt_count",      table: "pr_checks");
            migrationBuilder.DropColumn(name: "posted_at",          table: "pr_checks");
            migrationBuilder.DropColumn(name: "posting_started_at", table: "pr_checks");
            migrationBuilder.DropColumn(name: "head_sha",           table: "pr_checks");
            migrationBuilder.DropColumn(name: "annotations",        table: "pr_checks");
            migrationBuilder.DropColumn(name: "output_text",        table: "pr_checks");
            migrationBuilder.DropColumn(name: "output_summary",     table: "pr_checks");
            migrationBuilder.DropColumn(name: "details_url",        table: "pr_checks");
            migrationBuilder.DropColumn(name: "conclusion",         table: "pr_checks");
            migrationBuilder.DropColumn(name: "external_id",        table: "pr_checks");
            migrationBuilder.DropColumn(name: "check_run_id",       table: "pr_checks");
            migrationBuilder.DropColumn(name: "installation_id",    table: "pr_checks");
        }
    }
}
