using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Results;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Tests.Results;

/// <summary>Tests for <see cref="ResultsService"/> against in-memory SQLite.</summary>
public sealed class ResultsServiceTests
{
    // ── Test helpers ──────────────────────────────────────────────────────────

    private static async Task<(TestDbScope scope, AppDbContext db, ResultsService svc, Guid userId, Guid orgId)>
        BuildAsync()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();

        db.Users.Add(new User { Id = userId, Email = $"user-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "TestOrg",
            Slug = $"testorg-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var notifier = new FakeResultIngestedNotifier();
        var svc = new ResultsService(db, TimeProvider.System, notifier);
        return (scope, db, svc, userId, orgId);
    }

    private static UploadResultRequest ValidRequest(int itemCount = 1) =>
        new(
            CollectionName: "smoke-tests",
            RunAt: DateTime.UtcNow,
            DurationMs: 1234,
            PassCount: itemCount,
            FailCount: 0,
            SkippedCount: 0,
            TriggeredBy: "cli",
            GitSha: "abc123",
            Items: Enumerable.Range(0, itemCount)
                .Select(i => new UploadResultItemRequest($"test-{i}", "passed", 100, null))
                .ToList());

    // ── IngestAsync happy path + RBAC ─────────────────────────────────────────

    [Fact]
    public async Task IngestAsync_persists_result_and_items_for_org_member()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var request = ValidRequest(2);
            var (dto, error, _, _) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.None);
            dto.Should().NotBeNull();
            dto!.Id.Should().StartWith("res_");
            dto.PassCount.Should().Be(2);

            var resultCount = await db.Results.CountAsync();
            resultCount.Should().Be(1);

            var itemCount = await db.ResultItems.CountAsync();
            itemCount.Should().Be(2);
        }
    }

    [Fact]
    public async Task IngestAsync_returns_permission_denied_for_non_member_and_writes_nothing()
    {
        var (scope, db, svc, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var nonMemberId = Guid.NewGuid();
            db.Users.Add(new User { Id = nonMemberId, Email = "nonmember@test.com", CreatedAt = DateTime.UtcNow });
            await db.SaveChangesAsync();

            var (dto, error, _, _) = await svc.IngestAsync(nonMemberId, orgId, ValidRequest(), default);

            error.Should().Be(ResultError.PermissionDenied);
            dto.Should().BeNull();

            var resultCount = await db.Results.CountAsync();
            resultCount.Should().Be(0);
        }
    }

    [Fact]
    public async Task IngestAsync_invokes_notifier_after_save()
    {
        var scope = TestDb.CreateOpen();
        var db = scope.Db;
        await db.Database.MigrateAsync();

        var userId = Guid.NewGuid();
        var orgId = Guid.NewGuid();
        db.Users.Add(new User { Id = userId, Email = $"notif-{userId:N}@test.com", CreatedAt = DateTime.UtcNow });
        db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = "NotifOrg",
            Slug = $"notif-{orgId:N}"[..20],
            OwnerId = userId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        db.OrganizationMembers.Add(new OrganizationMember
        {
            OrgId = orgId,
            UserId = userId,
            Role = OrgRole.Owner,
            JoinedAt = DateTime.UtcNow,
        });
        await db.SaveChangesAsync();

        var notifier = new FakeResultIngestedNotifier();
        var svc = new ResultsService(db, TimeProvider.System, notifier);

        await using (scope)
        {
            var (dto, error, _, _) = await svc.IngestAsync(userId, orgId, ValidRequest(), default);

            error.Should().Be(ResultError.None);
            notifier.Calls.Should().HaveCount(1);
            notifier.Calls[0].orgId.Should().Be(orgId);
        }
    }

    // ── IngestAsync schema validation ─────────────────────────────────────────

    [Theory]
    [InlineData(null, null, 100L, 1, 0, "/collection_name")]
    [InlineData("", null, 100L, 1, 0, "/collection_name")]
    [InlineData("col", null, -1L, 1, 0, "/run_at")]
    [InlineData("col", "2026-01-01", -1L, 1, 0, "/duration_ms")]
    [InlineData("col", "2026-01-01", 100L, null, 0, "/pass_count")]
    [InlineData("col", "2026-01-01", 100L, 1, null, "/fail_count")]
    public async Task IngestAsync_returns_invalid_schema_with_pointer(
        string? collectionName,
        string? runAtStr,
        long? durationMs,
        int? passCount,
        int? failCount,
        string expectedPointer)
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            DateTime? runAt = runAtStr is null ? null : DateTime.Parse(runAtStr);
            var request = new UploadResultRequest(
                CollectionName: collectionName,
                RunAt: runAt,
                DurationMs: durationMs,
                PassCount: passCount,
                FailCount: failCount,
                SkippedCount: 0,
                TriggeredBy: null,
                GitSha: null,
                Items: new List<UploadResultItemRequest>());

            var (dto, error, _, pointer) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.InvalidSchema);
            dto.Should().BeNull();
            pointer.Should().Be(expectedPointer);
        }
    }

    [Fact]
    public async Task IngestAsync_returns_invalid_schema_when_items_is_null()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var request = new UploadResultRequest("col", DateTime.UtcNow, 100, 1, 0, 0, null, null, null);
            var (_, error, _, pointer) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.InvalidSchema);
            pointer.Should().Be("/items");
        }
    }

    [Fact]
    public async Task IngestAsync_rejects_negative_pass_count()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var request = new UploadResultRequest("col", DateTime.UtcNow, 100, -1, 0, 0, null, null, new List<UploadResultItemRequest>());
            var (_, error, _, pointer) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.InvalidSchema);
            pointer.Should().Be("/pass_count");
        }
    }

    [Fact]
    public async Task IngestAsync_rejects_item_with_unknown_status()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var request = new UploadResultRequest(
                "col", DateTime.UtcNow, 100, 1, 0, 0, null, null,
                new List<UploadResultItemRequest> { new("test-1", "flying", 100, null) });

            var (_, error, _, pointer) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.InvalidSchema);
            pointer.Should().Be("/items/0/status");
        }
    }

    [Fact]
    public async Task IngestAsync_rejects_more_than_max_items()
    {
        var (scope, _, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var items = Enumerable.Range(0, ResultsService.MaxItemsPerRequest + 1)
                .Select(i => new UploadResultItemRequest($"t-{i}", "passed", 1, null))
                .ToList();
            var request = new UploadResultRequest("col", DateTime.UtcNow, 100, 1, 0, 0, null, null, items);

            var (_, error, _, pointer) = await svc.IngestAsync(userId, orgId, request, default);

            error.Should().Be(ResultError.InvalidSchema);
            pointer.Should().Be("/items");
        }
    }

    // ── ListAsync ────────────────────────────────────────────────────────────

    [Fact]
    public async Task ListAsync_returns_newest_first_bounded_by_limit()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Insert 3 results with distinct timestamps
            for (var i = 0; i < 3; i++)
            {
                db.Results.Add(new Result
                {
                    Id = Guid.NewGuid(),
                    OrgId = orgId,
                    UploadedBy = userId,
                    CollectionName = $"run-{i}",
                    RunAt = DateTime.UtcNow.AddMinutes(-i),
                    DurationMs = 100,
                    PassCount = 1,
                    FailCount = 0,
                    SkippedCount = 0,
                    CreatedAt = DateTime.UtcNow.AddMinutes(-i),
                });
            }
            await db.SaveChangesAsync();

            var (results, error) = await svc.ListAsync(userId, orgId, limit: 2, default);

            error.Should().Be(ResultError.None);
            results.Should().HaveCount(2);
            // Newest-first: run-0 was inserted with the latest timestamp
            results[0].CollectionName.Should().Be("run-0");
            results[1].CollectionName.Should().Be("run-1");
        }
    }

    [Fact]
    public async Task ListAsync_returns_permission_denied_for_non_member()
    {
        var (scope, db, svc, _, orgId) = await BuildAsync();
        await using (scope)
        {
            var nonMemberId = Guid.NewGuid();
            db.Users.Add(new User { Id = nonMemberId, Email = "nm2@test.com", CreatedAt = DateTime.UtcNow });
            await db.SaveChangesAsync();

            var (results, error) = await svc.ListAsync(nonMemberId, orgId, limit: 10, default);

            error.Should().Be(ResultError.PermissionDenied);
            results.Should().BeEmpty();
        }
    }

    [Fact]
    public async Task ListAsync_clamps_limit_between_1_and_100()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Insert 5 results
            for (var i = 0; i < 5; i++)
            {
                db.Results.Add(new Result
                {
                    Id = Guid.NewGuid(),
                    OrgId = orgId,
                    UploadedBy = userId,
                    CollectionName = $"clamp-{i}",
                    RunAt = DateTime.UtcNow.AddMinutes(-i),
                    DurationMs = 100,
                    PassCount = 1,
                    FailCount = 0,
                    SkippedCount = 0,
                    CreatedAt = DateTime.UtcNow.AddMinutes(-i),
                });
            }
            await db.SaveChangesAsync();

            // limit=0 should clamp to 1
            var (r0, _) = await svc.ListAsync(userId, orgId, limit: 0, default);
            r0.Should().HaveCount(1);

            // limit=200 should clamp to 100 (we only have 5, so returns 5)
            var (r200, _) = await svc.ListAsync(userId, orgId, limit: 200, default);
            r200.Should().HaveCount(5);
        }
    }

    // ── GetDetailAsync ───────────────────────────────────────────────────────

    [Fact]
    public async Task GetDetailAsync_returns_detail_with_items_in_ordinal_order()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var resultId = Guid.NewGuid();
            db.Results.Add(new Result
            {
                Id = resultId,
                OrgId = orgId,
                UploadedBy = userId,
                CollectionName = "detail-test",
                RunAt = DateTime.UtcNow,
                DurationMs = 200,
                PassCount = 2,
                FailCount = 0,
                SkippedCount = 0,
                CreatedAt = DateTime.UtcNow,
            });
            // Add items in reverse order to test ordinal sorting
            db.ResultItems.Add(new ResultItem { Id = Guid.NewGuid(), ResultId = resultId, Ordinal = 1, Name = "B", Status = ResultStatus.Passed, DurationMs = 50 });
            db.ResultItems.Add(new ResultItem { Id = Guid.NewGuid(), ResultId = resultId, Ordinal = 0, Name = "A", Status = ResultStatus.Passed, DurationMs = 150 });
            await db.SaveChangesAsync();

            var (dto, error) = await svc.GetDetailAsync(userId, resultId, default);

            error.Should().Be(ResultError.None);
            dto.Should().NotBeNull();
            dto!.Items.Should().HaveCount(2);
            dto.Items[0].Name.Should().Be("A");
            dto.Items[1].Name.Should().Be("B");
        }
    }

    [Fact]
    public async Task GetDetailAsync_returns_not_found_for_cross_org_caller()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            // Insert result for original org
            var resultId = Guid.NewGuid();
            db.Results.Add(new Result
            {
                Id = resultId,
                OrgId = orgId,
                UploadedBy = userId,
                CollectionName = "cross-org",
                RunAt = DateTime.UtcNow,
                DurationMs = 100,
                PassCount = 1,
                FailCount = 0,
                SkippedCount = 0,
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();

            // Create a different user with no org membership
            var otherId = Guid.NewGuid();
            db.Users.Add(new User { Id = otherId, Email = "other@test.com", CreatedAt = DateTime.UtcNow });
            await db.SaveChangesAsync();

            var (dto, error) = await svc.GetDetailAsync(otherId, resultId, default);

            error.Should().Be(ResultError.NotFound);
            dto.Should().BeNull();
        }
    }

    [Fact]
    public async Task GetDetailAsync_returns_not_found_for_missing_id()
    {
        var (scope, _, svc, userId, _) = await BuildAsync();
        await using (scope)
        {
            var (dto, error) = await svc.GetDetailAsync(userId, Guid.NewGuid(), default);

            error.Should().Be(ResultError.NotFound);
            dto.Should().BeNull();
        }
    }

    // ── M16-019: method / request_url / path_template ingest ─────────────────

    [Fact]
    public async Task Ingest_persists_method_request_url_and_extracted_path_template()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var item = new UploadResultItemRequest(
                "POST /users/123/avatar", "failed", 100, "boom",
                Method: "POST", RequestUrl: "/users/123/avatar");

            var request = new UploadResultRequest(
                CollectionName: "http-tests",
                RunAt: DateTime.UtcNow,
                DurationMs: 100,
                PassCount: 0,
                FailCount: 1,
                SkippedCount: 0,
                TriggeredBy: null,
                GitSha: null,
                Items: [item]);

            var (dto, error, _, _) = await svc.IngestAsync(userId, orgId, request, default);
            error.Should().Be(ResultError.None);

            var saved = await db.ResultItems.FirstAsync();
            saved.Method.Should().Be("POST");
            saved.RequestUrl.Should().Be("/users/123/avatar");
            saved.PathTemplate.Should().Be("/users/{id}/avatar");
        }
    }

    [Fact]
    public async Task Ingest_with_no_method_or_url_persists_nulls()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var item = new UploadResultItemRequest("test-1", "passed", 100, null);

            var request = new UploadResultRequest(
                CollectionName: "legacy-tests",
                RunAt: DateTime.UtcNow,
                DurationMs: 100,
                PassCount: 1,
                FailCount: 0,
                SkippedCount: 0,
                TriggeredBy: null,
                GitSha: null,
                Items: [item]);

            var (dto, error, _, _) = await svc.IngestAsync(userId, orgId, request, default);
            error.Should().Be(ResultError.None);

            var saved = await db.ResultItems.FirstAsync();
            saved.Method.Should().BeNull();
            saved.RequestUrl.Should().BeNull();
            saved.PathTemplate.Should().BeNull();
        }
    }

    [Fact]
    public async Task Ingest_uppercases_method_before_storing()
    {
        var (scope, db, svc, userId, orgId) = await BuildAsync();
        await using (scope)
        {
            var item = new UploadResultItemRequest(
                "get /health", "passed", 50, null,
                Method: "get", RequestUrl: "/health");

            var request = new UploadResultRequest(
                CollectionName: "method-case-test",
                RunAt: DateTime.UtcNow,
                DurationMs: 50,
                PassCount: 1,
                FailCount: 0,
                SkippedCount: 0,
                TriggeredBy: null,
                GitSha: null,
                Items: [item]);

            await svc.IngestAsync(userId, orgId, request, default);

            var saved = await db.ResultItems.FirstAsync();
            saved.Method.Should().Be("GET");
        }
    }
}
