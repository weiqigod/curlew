using ApiTool.Backend.Audit;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>
/// Unit tests for <see cref="AuditLogQueryService.StreamForExportAsync"/> and
/// <see cref="AuditLogQueryService.EnsureExporterAuthorisedAsync"/>.
/// Uses an in-memory SQLite database for realistic EF Core query behaviour.
/// </summary>
public sealed class AuditLogQueryServiceStreamTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly AuditLogQueryService _svc;

    private readonly Guid _orgId = Guid.NewGuid();
    private readonly Guid _ownerId = Guid.NewGuid();
    private readonly Guid _adminId = Guid.NewGuid();
    private readonly Guid _memberId = Guid.NewGuid();

    public AuditLogQueryServiceStreamTests()
    {
        _scope = TestDb.CreateOpen();
        _scope.Db.Database.EnsureCreated();
        _svc = new AuditLogQueryService(_scope.Db, new RoleResolver(_scope.Db));
        SeedBaseDataAsync().GetAwaiter().GetResult();
    }

    private async Task SeedBaseDataAsync()
    {
        _scope.Db.Users.AddRange(
            new User { Id = _ownerId, Email = $"owner-{_ownerId:N}@test.com", CreatedAt = DateTime.UtcNow },
            new User { Id = _adminId, Email = $"admin-{_adminId:N}@test.com", CreatedAt = DateTime.UtcNow },
            new User { Id = _memberId, Email = $"member-{_memberId:N}@test.com", CreatedAt = DateTime.UtcNow }
        );
        _scope.Db.Organizations.Add(new Organization
        {
            Id = _orgId,
            Name = "Stream Test Org",
            Slug = $"st-{_orgId:N}"[..20],
            OwnerId = _ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        _scope.Db.OrganizationMembers.AddRange(
            new OrganizationMember { OrgId = _orgId, UserId = _ownerId, Role = OrgRole.Owner, JoinedAt = DateTime.UtcNow },
            new OrganizationMember { OrgId = _orgId, UserId = _adminId, Role = OrgRole.Admin, JoinedAt = DateTime.UtcNow },
            new OrganizationMember { OrgId = _orgId, UserId = _memberId, Role = OrgRole.Member, JoinedAt = DateTime.UtcNow }
        );
        await _scope.Db.SaveChangesAsync();
    }

    // ── StreamForExportAsync ──────────────────────────────────────────────────

    [Fact]
    public async Task StreamForExportAsync_returns_all_rows_no_cap()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        for (var i = 0; i < 350; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "cap.test", CreatedAt = DateTime.UtcNow.AddSeconds(-i), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var filter = new AuditLogFilter();
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(350);
    }

    [Fact]
    public async Task StreamForExportAsync_applies_event_type_filter()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        for (var i = 0; i < 175; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "member.invited", CreatedAt = DateTime.UtcNow.AddSeconds(-i), Success = true,
            });
        for (var i = 0; i < 175; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "org.other", CreatedAt = DateTime.UtcNow.AddSeconds(-i - 1000), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var filter = new AuditLogFilter(EventType: "member.invited");
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(175);
        rows.Should().OnlyContain(r => r.EventType == "member.invited");
    }

    [Fact]
    public async Task StreamForExportAsync_applies_user_id_filter()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        var otherUser = Guid.NewGuid();
        for (var i = 0; i < 100; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "uid.owner", CreatedAt = DateTime.UtcNow.AddSeconds(-i), Success = true,
            });
        for (var i = 0; i < 50; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = otherUser,
                EventType = "uid.other", CreatedAt = DateTime.UtcNow.AddSeconds(-i - 500), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var filter = new AuditLogFilter(UserId: _ownerId);
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(100);
        rows.Should().OnlyContain(r => r.UserId == _ownerId.ToString("N"));
    }

    [Fact]
    public async Task StreamForExportAsync_applies_from_filter()
    {
        // Use a dedicated org so other test methods' seeded rows don't interfere.
        var localOrgId = await SeedIsolatedOrgAsync();
        var cutoff = new DateTime(2025, 6, 1, 12, 0, 0, DateTimeKind.Utc);

        // 50 rows after cutoff-1min (within range for From = cutoff)
        for (var i = 0; i < 50; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "from.test", CreatedAt = cutoff.AddSeconds(i + 1), Success = true,
            });
        // 200 rows 2 hours before cutoff (outside range)
        for (var i = 0; i < 200; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "from.old", CreatedAt = cutoff.AddHours(-2).AddSeconds(-i), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var filter = new AuditLogFilter(From: cutoff);
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(50);
    }

    [Fact]
    public async Task StreamForExportAsync_applies_to_filter()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        var cutoff = new DateTime(2025, 6, 1, 12, 0, 0, DateTimeKind.Utc);

        // 200 rows 2 hours before cutoff (within to-range when To = cutoff-1h)
        for (var i = 0; i < 200; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "to.old", CreatedAt = cutoff.AddHours(-2).AddSeconds(-i), Success = true,
            });
        // 50 rows after cutoff (outside to-range)
        for (var i = 0; i < 50; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "to.new", CreatedAt = cutoff.AddSeconds(i + 1), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var filter = new AuditLogFilter(To: cutoff.AddHours(-1));
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(200);
    }

    private async Task<Guid> SeedIsolatedOrgAsync()
    {
        var orgId = Guid.NewGuid();
        _scope.Db.Organizations.Add(new Organization
        {
            Id = orgId,
            Name = $"Isolated-{orgId:N}"[..30],
            Slug = $"iso-{orgId:N}"[..20],
            OwnerId = _ownerId,
            Status = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await _scope.Db.SaveChangesAsync();
        return orgId;
    }

    [Fact]
    public async Task StreamForExportAsync_orders_newest_first()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        var baseTime = new DateTime(2025, 1, 1, 0, 0, 0, DateTimeKind.Utc);
        for (var i = 0; i < 10; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "order.test", CreatedAt = baseTime.AddHours(i), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, new AuditLogFilter(), CancellationToken.None))
            rows.Add(dto);

        rows.Select(r => r.CreatedAt).Should().BeInDescendingOrder();
    }

    [Fact]
    public async Task StreamForExportAsync_ignores_limit_parameter()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        for (var i = 0; i < 300; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "limit.test", CreatedAt = DateTime.UtcNow.AddSeconds(-i), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        // limit=5 on the paginated path would cap at 5, but export ignores it
        var filter = new AuditLogFilter(Limit: 5);
        var rows = new List<AuditLogEntryDto>();
        await foreach (var dto in _svc.StreamForExportAsync(localOrgId, filter, CancellationToken.None))
            rows.Add(dto);

        rows.Should().HaveCount(300);
    }

    [Fact]
    public async Task StreamForExportAsync_propagates_cancellation()
    {
        var localOrgId = await SeedIsolatedOrgAsync();
        for (var i = 0; i < 50; i++)
            _scope.Db.OrganizationAuditLog.Add(new OrganizationAuditLogEntry
            {
                Id = Guid.NewGuid(), OrgId = localOrgId, ActorId = _ownerId,
                EventType = "cancel.test", CreatedAt = DateTime.UtcNow.AddSeconds(-i), Success = true,
            });
        await _scope.Db.SaveChangesAsync();

        using var cts = new CancellationTokenSource();
        cts.Cancel();

        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var _ in _svc.StreamForExportAsync(localOrgId, new AuditLogFilter(), cts.Token))
            {
                // Should not get here
            }
        });
    }

    // ── EnsureExporterAuthorisedAsync ─────────────────────────────────────────

    [Fact]
    public async Task EnsureExporterAuthorisedAsync_returns_None_for_owner_role()
    {
        var result = await _svc.EnsureExporterAuthorisedAsync(_ownerId, _orgId, CancellationToken.None);
        result.Should().Be(AuditLogError.None);
    }

    [Fact]
    public async Task EnsureExporterAuthorisedAsync_returns_None_for_admin_role()
    {
        var result = await _svc.EnsureExporterAuthorisedAsync(_adminId, _orgId, CancellationToken.None);
        result.Should().Be(AuditLogError.None);
    }

    [Fact]
    public async Task EnsureExporterAuthorisedAsync_returns_PermissionDenied_for_member_role()
    {
        var result = await _svc.EnsureExporterAuthorisedAsync(_memberId, _orgId, CancellationToken.None);
        result.Should().Be(AuditLogError.PermissionDenied);
    }

    [Fact]
    public async Task EnsureExporterAuthorisedAsync_returns_PermissionDenied_for_nonmember()
    {
        var stranger = Guid.NewGuid();
        var result = await _svc.EnsureExporterAuthorisedAsync(stranger, _orgId, CancellationToken.None);
        result.Should().Be(AuditLogError.PermissionDenied);
    }

    public async ValueTask DisposeAsync() => await _scope.DisposeAsync();
}
