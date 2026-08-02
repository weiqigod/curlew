using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Data;

/// <summary>
/// Schema tests verifying that the 7 SetNull anonymisation columns are widened
/// to nullable Guid? and that actor_id on organization_audit_log is renamed to
/// actor_id (snake_case) with nullable column semantics. (M18-006, Step 1).
/// </summary>
public sealed class AnonymisationSchemaTests
{
    [Fact]
    public async Task Audit_log_actor_id_column_is_named_actor_id_and_nullable()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        // pragma_table_info returns: cid, name, type, notnull, dflt_value, pk
        // We project name column from rows where name='actor_id' AND notnull=0.
        var sql = "SELECT name FROM pragma_table_info('organization_audit_log') WHERE name = 'actor_id' AND [notnull] = 0";
        var cols = await scope.Db.Database.SqlQueryRaw<string>(sql).ToListAsync();
        cols.Should().ContainSingle(because: "actor_id must be a nullable column in organization_audit_log");
    }

    [Theory]
    [InlineData("notification_rules", "CreatedBy")]
    [InlineData("organization_custom_roles", "CreatedBy")]
    [InlineData("schedules", "CreatedBy")]
    [InlineData("coordinator_jobs", "CreatedBy")]
    [InlineData("team_vaults", "created_by")]
    [InlineData("team_vaults", "updated_by")]
    public async Task Anonymisable_creator_columns_are_nullable(string table, string column)
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        // [notnull] = 0 means the column IS nullable
        var sql = $"SELECT [notnull] AS Value FROM pragma_table_info('{table}') WHERE name = '{column}'";
        var notnull = await scope.Db.Database.SqlQueryRaw<int>(sql).SingleAsync();
        notnull.Should().Be(0, $"{table}.{column} must be nullable for [GdprAnonymise(SetNull)]");
    }
}
