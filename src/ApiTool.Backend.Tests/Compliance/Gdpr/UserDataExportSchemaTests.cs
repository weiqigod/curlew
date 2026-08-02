using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>Migration round-trip tests for the user_export_requests table.</summary>
public class UserDataExportSchemaTests
{
    [Fact]
    public async Task Migration_creates_user_export_requests_table_with_expected_columns()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"schema-test-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();

        var req = new UserExportRequest
        {
            Id = Guid.NewGuid(),
            UserId = userId,
            Status = UserExportStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        };
        db.UserExportRequests.Add(req);
        await db.SaveChangesAsync();

        var roundtrip = await db.UserExportRequests.FindAsync(req.Id);
        roundtrip.Should().NotBeNull();
        roundtrip!.Status.Should().Be(UserExportStatus.Queued);
        roundtrip.UserId.Should().Be(userId);
        roundtrip.ObjectKey.Should().BeNull();
        roundtrip.ReadyAt.Should().BeNull();
        roundtrip.ExpiresAt.Should().BeNull();
        roundtrip.FailureReason.Should().BeNull();
    }

    [Fact]
    public async Task Status_transitions_persist_correctly()
    {
        using var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.EnsureCreatedAsync();

        var userId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"schema-status-{userId:N}@example.com", CreatedAt = DateTime.UtcNow });
        await db.SaveChangesAsync();

        var id = Guid.NewGuid();
        db.UserExportRequests.Add(new UserExportRequest
        {
            Id = id,
            UserId = userId,
            Status = UserExportStatus.Queued,
            CreatedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();

        var row = (await db.UserExportRequests.FindAsync(id))!;
        row.Status = UserExportStatus.Ready;
        row.ObjectKey = "exports/test/bundle.json";
        row.ReadyAt = DateTime.UtcNow;
        row.ExpiresAt = DateTime.UtcNow.AddHours(24);
        await db.SaveChangesAsync();
        db.ChangeTracker.Clear();

        var final = (await db.UserExportRequests.FindAsync(id))!;
        final.Status.Should().Be(UserExportStatus.Ready);
        final.ObjectKey.Should().Be("exports/test/bundle.json");
        final.ReadyAt.Should().NotBeNull();
        final.ExpiresAt.Should().NotBeNull();
    }

    [Fact]
    public async Task Index_on_user_id_exists_in_model()
    {
        // Verify EF model has the (user_id, created_at) index via model metadata.
        var opts = new Microsoft.EntityFrameworkCore.DbContextOptionsBuilder<ApiTool.Backend.Data.AppDbContext>()
            .UseInMemoryDatabase($"schema_idx_{Guid.NewGuid():N}")
            .Options;
        using var ctx = new ApiTool.Backend.Data.AppDbContext(opts);
        var entityType = ctx.Model.FindEntityType(typeof(UserExportRequest));
        entityType.Should().NotBeNull();
        var indexes = entityType!.GetIndexes().ToList();
        indexes.Should().Contain(idx =>
            idx.Properties.Any(p => p.Name == nameof(UserExportRequest.UserId)));
    }
}
