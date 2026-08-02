using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Results.Dashboard;

/// <summary>Tests that verify the new schema columns on <see cref="ResultItem"/>.</summary>
public sealed class ResultItemSchemaTests
{
    [Fact]
    public void Method_request_url_path_template_are_nullable_strings()
    {
        var props = typeof(ResultItem).GetProperties();

        var method = props.Single(p => p.Name == "Method");
        var requestUrl = props.Single(p => p.Name == "RequestUrl");
        var pathTemplate = props.Single(p => p.Name == "PathTemplate");

        Assert.Equal(typeof(string), method.PropertyType);
        Assert.Equal(typeof(string), requestUrl.PropertyType);
        Assert.Equal(typeof(string), pathTemplate.PropertyType);

        // All must be nullable — verify via reflection that they aren't value types
        // (string is a reference type; null-annotations are not enforced at runtime
        // but the absence of [Required] and the presence of ? in the type system is
        // validated by the migration test below via actual null writes).
        Assert.True(method.PropertyType.IsClass);
        Assert.True(requestUrl.PropertyType.IsClass);
        Assert.True(pathTemplate.PropertyType.IsClass);
    }

    [Fact]
    public async Task Migration_creates_columns_and_allows_null_values()
    {
        await using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new ApiTool.Backend.Data.Entities.User
        {
            Id = userId,
            Email = $"schema-test-{userId:N}@test.com",
            CreatedAt = DateTime.UtcNow,
        });
        db.Organizations.Add(new ApiTool.Backend.Data.Entities.Organization
        {
            Id = orgId,
            Name = "SchemaTestOrg",
            Slug = $"schemaorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = ApiTool.Backend.Data.Entities.OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        var resultId = Guid.NewGuid();
        db.Results.Add(new ApiTool.Backend.Data.Entities.Result
        {
            Id = resultId,
            OrgId = orgId,
            UploadedBy = userId,
            CollectionName = "schema-test",
            RunAt = DateTime.UtcNow,
            DurationMs = 100,
            PassCount = 1,
            FailCount = 0,
            SkippedCount = 0,
            CreatedAt = DateTime.UtcNow,
        });

        // Write a ResultItem with all three new nullable columns explicitly null
        db.ResultItems.Add(new ResultItem
        {
            Id = Guid.NewGuid(),
            ResultId = resultId,
            Ordinal = 0,
            Name = "null-fields-test",
            Status = ApiTool.Backend.Data.Entities.ResultStatus.Passed,
            DurationMs = 50,
            Method = null,
            RequestUrl = null,
            PathTemplate = null,
        });

        // Write a ResultItem with all three columns populated
        db.ResultItems.Add(new ResultItem
        {
            Id = Guid.NewGuid(),
            ResultId = resultId,
            Ordinal = 1,
            Name = "populated-fields-test",
            Status = ApiTool.Backend.Data.Entities.ResultStatus.Failed,
            DurationMs = 75,
            Method = "GET",
            RequestUrl = "/users/123",
            PathTemplate = "/users/{id}",
        });

        await db.SaveChangesAsync();

        // Reload and verify
        var items = await db.ResultItems
            .Where(i => i.ResultId == resultId)
            .OrderBy(i => i.Ordinal)
            .ToListAsync();

        Assert.Equal(2, items.Count);

        var nullItem = items[0];
        Assert.Null(nullItem.Method);
        Assert.Null(nullItem.RequestUrl);
        Assert.Null(nullItem.PathTemplate);

        var populatedItem = items[1];
        Assert.Equal("GET", populatedItem.Method);
        Assert.Equal("/users/123", populatedItem.RequestUrl);
        Assert.Equal("/users/{id}", populatedItem.PathTemplate);
    }
}
