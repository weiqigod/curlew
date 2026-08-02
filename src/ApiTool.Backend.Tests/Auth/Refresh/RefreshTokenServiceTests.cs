// Tests for RefreshTokenService — rotate-or-revoke business logic.
// Uses SQLite-in-memory for real transaction support.
// Refs docs/SPECIFICATION.md:7901-7944 (rotation semantics).
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Tests.Notifications.Email;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Auth.Refresh;

public sealed class RefreshTokenServiceTests : IAsyncDisposable
{
    private readonly TestDbScope _scope;
    private readonly AppDbContext _db;
    private readonly RefreshTokenIssuer _issuer;
    private readonly RecordingEmailQueue _emailQueue;
    private readonly RefreshTokenService _sut;

    public RefreshTokenServiceTests()
    {
        _scope      = TestDb.CreateOpen();
        _db         = _scope.Db;
        _issuer     = new RefreshTokenIssuer(TimeProvider.System);
        _emailQueue = new RecordingEmailQueue();
        _sut        = new RefreshTokenService(_db, _emailQueue, TimeProvider.System,
            NullLogger<RefreshTokenService>.Instance);
    }

    public async ValueTask DisposeAsync()
    {
        await _scope.DisposeAsync();
    }

    // ── Setup helpers ─────────────────────────────────────────────────────────

    private async Task<(string plaintext, RefreshToken row, User user)> SeedRootTokenAsync(
        DateTime? rootIssuedAt = null)
    {
        await _db.Database.MigrateAsync();
        var user = new User { Id = Guid.NewGuid(), Email = "u@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);

        var issuedAt = rootIssuedAt ?? DateTime.UtcNow;
        var deviceId = Guid.NewGuid();
        var familyId = Guid.NewGuid();
        var (plaintext, row) = _issuer.Mint(user.Id, deviceId, familyId,
            parentId: null, familyRootIssuedAt: issuedAt, clientIp: null, userAgent: null);

        // Override IssuedAt if needed (e.g., testing the 365d clamping)
        if (rootIssuedAt.HasValue)
            row.IssuedAt = rootIssuedAt.Value;

        _db.RefreshTokens.Add(row);
        await _db.SaveChangesAsync();
        return (plaintext, row, user);
    }

    // ── Tests ─────────────────────────────────────────────────────────────────

    [Fact]
    public async Task RotateAsync_happy_path_marks_old_rotated_and_inserts_new_row_with_parent_id()
    {
        // Behavior #2 from task YAML
        var (plaintext, oldRow, user) = await SeedRootTokenAsync();

        var result = await _sut.RotateAsync(plaintext, oldRow.DeviceId, "1.2.3.4", "ua", CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Success);
        result.NewRow.Should().NotBeNull();
        result.NewPlaintextToken.Should().NotBeNullOrEmpty();
        result.UserId.Should().Be(user.Id);

        // Old row should be rotated
        _db.ChangeTracker.Clear();
        var refreshedOld = await _db.RefreshTokens.FindAsync(oldRow.Id);
        refreshedOld!.RotatedAt.Should().NotBeNull();

        // New row should exist with parent and family linkage
        var newRow = await _db.RefreshTokens
            .FirstOrDefaultAsync(r => r.Id == result.NewRow!.Id);
        newRow.Should().NotBeNull();
        newRow!.FamilyId.Should().Be(oldRow.FamilyId);
        newRow.ParentId.Should().Be(oldRow.Id);
    }

    [Fact]
    public async Task RotateAsync_new_row_expires_at_clamped_to_365_days_from_family_root()
    {
        // Behavior #2 lifetime piece
        // Family root issued 350 days ago → absolute deadline ≈ 15 days from now.
        var rootIssuedAt = DateTime.UtcNow.AddDays(-350);
        var (plaintext, oldRow, _) = await SeedRootTokenAsync(rootIssuedAt: rootIssuedAt);

        var result = await _sut.RotateAsync(plaintext, oldRow.DeviceId, null, null, CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Success);
        var expectedExpiry = rootIssuedAt.AddDays(365);
        result.NewRow!.ExpiresAt.Should().BeCloseTo(expectedExpiry, TimeSpan.FromSeconds(10));
    }

    [Fact]
    public async Task RotateAsync_reuse_revokes_entire_family_and_enqueues_account_security_alert()
    {
        // Behavior #3
        var (plaintext, oldRow, _) = await SeedRootTokenAsync();

        // Mark the root token as already rotated (simulate prior use)
        oldRow.RotatedAt = DateTime.UtcNow.AddMinutes(-5);
        _db.RefreshTokens.Update(oldRow);
        await _db.SaveChangesAsync();
        _db.ChangeTracker.Clear();

        // Also seed a child token in same family
        var childDeviceId = oldRow.DeviceId;
        var childFamilyId = oldRow.FamilyId;
        var childRow      = new RefreshToken
        {
            Id         = Guid.NewGuid(),
            TokenHash  = RefreshTokenIssuer.Hash("child-plaintext"),
            UserId     = oldRow.UserId,
            DeviceId   = childDeviceId,
            FamilyId   = childFamilyId,
            ParentId   = oldRow.Id,
            IssuedAt   = DateTime.UtcNow.AddMinutes(-5),
            ExpiresAt  = DateTime.UtcNow.AddDays(90),
        };
        _db.RefreshTokens.Add(childRow);
        await _db.SaveChangesAsync();
        _db.ChangeTracker.Clear();

        // Attempt to reuse the already-rotated root token
        var result = await _sut.RotateAsync(plaintext, oldRow.DeviceId, "1.2.3.4", "ua", CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Reused);

        // Every row in the family must be revoked
        _db.ChangeTracker.Clear();
        var familyRows = await _db.RefreshTokens
            .Where(r => r.FamilyId == oldRow.FamilyId)
            .ToListAsync();

        familyRows.Should().AllSatisfy(r =>
        {
            r.RevokedAt.Should().NotBeNull();
            r.RevokeReason.Should().Be("reuse_detected");
        });

        // Email must be enqueued
        _emailQueue.Messages.Should().HaveCount(1);
        var email = _emailQueue.Messages[0];
        email.TemplateSlug.Should().Be("account_security_alert");
        email.Variables.Keys.Should().Contain(new[] { "first_name", "event_time", "event_ip", "relogin_url" });
    }

    [Fact]
    public async Task RotateAsync_with_mismatched_device_id_returns_DeviceMismatch_and_does_not_rotate()
    {
        // Behavior #4
        var (plaintext, oldRow, _) = await SeedRootTokenAsync();
        var wrongDevice = Guid.NewGuid();

        var result = await _sut.RotateAsync(plaintext, wrongDevice, null, null, CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.DeviceMismatch);

        // Old row must NOT be rotated
        _db.ChangeTracker.Clear();
        var refreshedOld = await _db.RefreshTokens.FindAsync(oldRow.Id);
        refreshedOld!.RotatedAt.Should().BeNull();
    }

    [Fact]
    public async Task RotateAsync_past_expires_at_returns_Expired()
    {
        // Behavior #5
        await _db.Database.MigrateAsync();
        var user = new User { Id = Guid.NewGuid(), Email = "e@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);
        var deviceId = Guid.NewGuid();
        var plaintext = "expired-token-aaaaaaaaaaaaaaaaaaaaaaaaaaa";
        var row = new RefreshToken
        {
            Id        = Guid.NewGuid(),
            TokenHash = RefreshTokenIssuer.Hash(plaintext),
            UserId    = user.Id,
            DeviceId  = deviceId,
            FamilyId  = Guid.NewGuid(),
            IssuedAt  = DateTime.UtcNow.AddDays(-400),
            ExpiresAt = DateTime.UtcNow.AddDays(-10), // already expired
        };
        _db.RefreshTokens.Add(row);
        await _db.SaveChangesAsync();

        var result = await _sut.RotateAsync(plaintext, deviceId, null, null, CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Expired);
    }

    [Fact]
    public async Task RotateAsync_with_unknown_token_returns_NotFound()
    {
        await _db.Database.MigrateAsync();
        var result = await _sut.RotateAsync(
            "unknown-token-aaaaaaaaaaaaaaaaaaaaaaaa",
            Guid.NewGuid(), null, null, CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.NotFound);
    }

    [Fact]
    public async Task RotateAsync_with_revoked_token_returns_Revoked()
    {
        await _db.Database.MigrateAsync();
        var user = new User { Id = Guid.NewGuid(), Email = "r@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);
        var deviceId  = Guid.NewGuid();
        var plaintext = "revoked-token-aaaaaaaaaaaaaaaaaaaaaaaaaaa";
        var row = new RefreshToken
        {
            Id          = Guid.NewGuid(),
            TokenHash   = RefreshTokenIssuer.Hash(plaintext),
            UserId      = user.Id,
            DeviceId    = deviceId,
            FamilyId    = Guid.NewGuid(),
            IssuedAt    = DateTime.UtcNow.AddDays(-5),
            ExpiresAt   = DateTime.UtcNow.AddDays(85),
            RevokedAt   = DateTime.UtcNow.AddMinutes(-10),
            RevokeReason = "reuse_detected",
        };
        _db.RefreshTokens.Add(row);
        await _db.SaveChangesAsync();

        var result = await _sut.RotateAsync(plaintext, deviceId, null, null, CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Revoked);
        // No additional email should be sent for an already-revoked token
        _emailQueue.Messages.Should().BeEmpty();
    }

    [Fact]
    public async Task RotateAsync_with_missing_family_root_falls_back_gracefully_and_succeeds()
    {
        // Finding #1: when a child token's FamilyId row is missing (data-integrity anomaly),
        // RotateHappyPathAsync must fall back to the current row's IssuedAt and NOT throw.
        //
        // We create a parent row so the ParentId FK is satisfied, then we insert the child
        // pointing at that parent as its ParentId. The child's FamilyId, however, is a
        // non-FK UUID that points to no row — this triggers the family-root lookup to return null.
        await _db.Database.MigrateAsync();
        var user = new User { Id = Guid.NewGuid(), Email = "missing-root@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);

        var deviceId   = Guid.NewGuid();
        var familyId   = Guid.NewGuid(); // This FamilyId has NO corresponding row in refresh_tokens

        // Insert the parent row (needed to satisfy the ParentId FK)
        var parentPlaintext = "parent-tok-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
        var parentRow = new RefreshToken
        {
            Id        = Guid.NewGuid(),
            TokenHash = RefreshTokenIssuer.Hash(parentPlaintext),
            UserId    = user.Id,
            DeviceId  = deviceId,
            FamilyId  = familyId,
            ParentId  = null,
            IssuedAt  = DateTime.UtcNow.AddDays(-10),
            ExpiresAt = DateTime.UtcNow.AddDays(80),
            RotatedAt = DateTime.UtcNow.AddMinutes(-5), // mark as rotated so we can use child
        };
        _db.RefreshTokens.Add(parentRow);

        // The child row has ParentId = parentRow.Id (FK satisfied),
        // but FamilyId is set to a UUID with no matching row (simulates missing root in family lookup)
        var childPlaintext = "child-token-orphaned-aaaaaaaaaaaaaaaaaaaaa";
        var missingRootFamilyId = Guid.NewGuid(); // no row with Id = this value
        var childRow = new RefreshToken
        {
            Id        = Guid.NewGuid(),
            TokenHash = RefreshTokenIssuer.Hash(childPlaintext),
            UserId    = user.Id,
            DeviceId  = deviceId,
            FamilyId  = missingRootFamilyId, // FamilyId pointing to non-existent row
            ParentId  = parentRow.Id,         // non-null: marks this row as a child
            IssuedAt  = DateTime.UtcNow.AddDays(-5),
            ExpiresAt = DateTime.UtcNow.AddDays(85),
        };
        _db.RefreshTokens.Add(childRow);
        await _db.SaveChangesAsync();

        // Act — should not throw; service falls back to row.IssuedAt for clamping
        var result = await _sut.RotateAsync(childPlaintext, deviceId, "1.2.3.4", "ua", CancellationToken.None);

        result.Outcome.Should().Be(RotateOutcome.Success);
        result.NewRow.Should().NotBeNull();
        // New row lifetime should be based on the fallback (childRow.IssuedAt) — ~85 days from now
        result.NewRow!.ExpiresAt.Should().BeAfter(DateTime.UtcNow.AddDays(84));
    }

    [Fact]
    public async Task RevokeEntireFamilyAsync_when_user_row_deleted_does_not_throw_and_enqueues_email()
    {
        // Finding #6: when the user row is deleted before RevokeEntireFamilyAsync runs,
        // the service should not throw and the email should still be enqueued with relogin_url.
        //
        // We insert a user + token, mark the token as rotated (triggers reuse path),
        // then delete the user row before calling RotateAsync. The cascade on refresh_tokens
        // is DeleteBehavior.Cascade, so we disable FK checks to delete user while keeping token.
        await _db.Database.MigrateAsync();

        var user = new User { Id = Guid.NewGuid(), Email = "ghost-user@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);

        var deviceId  = Guid.NewGuid();
        var plaintext = "ghost-user-tok-aaaaaaaaaaaaaaaaaaaaaaaaaaa";
        var row = new RefreshToken
        {
            Id        = Guid.NewGuid(),
            TokenHash = RefreshTokenIssuer.Hash(plaintext),
            UserId    = user.Id,
            DeviceId  = deviceId,
            FamilyId  = Guid.NewGuid(),
            IssuedAt  = DateTime.UtcNow.AddDays(-5),
            ExpiresAt = DateTime.UtcNow.AddDays(85),
            RotatedAt = DateTime.UtcNow.AddMinutes(-5), // already rotated → triggers reuse path
        };
        _db.RefreshTokens.Add(row);
        await _db.SaveChangesAsync();
        _db.ChangeTracker.Clear();

        // Remove the user via raw SQL bypassing FK (simulates the deleted-user race).
        // SQLite: disable FK checks for this delete so the token row orphaned-user scenario is testable.
        await _db.Database.ExecuteSqlRawAsync("PRAGMA foreign_keys = OFF");
        await _db.Database.ExecuteSqlAsync($"DELETE FROM users WHERE id = {user.Id}");
        await _db.Database.ExecuteSqlRawAsync("PRAGMA foreign_keys = ON");
        _db.ChangeTracker.Clear();

        // Act — reuse path → calls RevokeEntireFamilyAsync → user lookup returns null
        var act = () => _sut.RotateAsync(plaintext, deviceId, "1.2.3.4", "ua", CancellationToken.None);
        await act.Should().NotThrowAsync();

        // Email must still be enqueued, and relogin_url must be present
        _emailQueue.Messages.Should().HaveCount(1);
        var email = _emailQueue.Messages[0];
        email.TemplateSlug.Should().Be("account_security_alert");
        email.Variables.Should().ContainKey("relogin_url");
        // When user is null, To defaults to empty string — service must not throw on empty To
        email.To.Should().Be(string.Empty);
    }

    // ── RevokeAllFamiliesForUserAsync ─────────────────────────────────────────

    [Fact]
    public async Task RevokeAllFamiliesForUserAsync_marks_every_non_revoked_row_for_the_user()
    {
        await _db.Database.MigrateAsync();

        var userA = new User { Id = Guid.NewGuid(), Email = "a@example.com", CreatedAt = DateTime.UtcNow };
        var userB = new User { Id = Guid.NewGuid(), Email = "b@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.AddRange(userA, userB);

        var deviceId = Guid.NewGuid();
        var familyA1 = Guid.NewGuid();
        var familyA2 = Guid.NewGuid();
        var rowA1 = new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = userA.Id, DeviceId = deviceId, FamilyId = familyA1,
            TokenHash = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32],
            IssuedAt = DateTime.UtcNow.AddDays(-1), ExpiresAt = DateTime.UtcNow.AddDays(89),
        };
        var rowA2 = new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = userA.Id, DeviceId = deviceId, FamilyId = familyA2,
            TokenHash = [2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 1],
            IssuedAt = DateTime.UtcNow.AddDays(-2), ExpiresAt = DateTime.UtcNow.AddDays(88),
        };
        var rowB = new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = userB.Id, DeviceId = deviceId, FamilyId = Guid.NewGuid(),
            TokenHash = [3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 1, 2],
            IssuedAt = DateTime.UtcNow.AddDays(-1), ExpiresAt = DateTime.UtcNow.AddDays(89),
        };
        _db.RefreshTokens.AddRange(rowA1, rowA2, rowB);
        await _db.SaveChangesAsync();

        var count = await _sut.RevokeAllFamiliesForUserAsync(userA.Id, "password_reset", CancellationToken.None);

        count.Should().Be(2);

        _db.ChangeTracker.Clear();
        var a1 = await _db.RefreshTokens.FindAsync(rowA1.Id);
        var a2 = await _db.RefreshTokens.FindAsync(rowA2.Id);
        var b  = await _db.RefreshTokens.FindAsync(rowB.Id);

        a1!.RevokedAt.Should().NotBeNull();
        a1.RevokeReason.Should().Be("password_reset");
        a2!.RevokedAt.Should().NotBeNull();
        a2.RevokeReason.Should().Be("password_reset");
        b!.RevokedAt.Should().BeNull();
    }

    [Fact]
    public async Task RevokeAllFamiliesForUserAsync_returns_zero_when_user_has_no_rows()
    {
        await _db.Database.MigrateAsync();

        var n = await _sut.RevokeAllFamiliesForUserAsync(Guid.NewGuid(), "password_reset", CancellationToken.None);

        n.Should().Be(0);
    }

    [Fact]
    public async Task RevokeAllFamiliesForUserAsync_idempotent_when_called_twice()
    {
        await _db.Database.MigrateAsync();

        var user = new User { Id = Guid.NewGuid(), Email = "idem@example.com", CreatedAt = DateTime.UtcNow };
        _db.Users.Add(user);
        var row = new RefreshToken
        {
            Id = Guid.NewGuid(), UserId = user.Id, DeviceId = Guid.NewGuid(), FamilyId = Guid.NewGuid(),
            TokenHash = [5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 1, 2, 3, 4],
            IssuedAt = DateTime.UtcNow.AddDays(-1), ExpiresAt = DateTime.UtcNow.AddDays(89),
        };
        _db.RefreshTokens.Add(row);
        await _db.SaveChangesAsync();

        var first  = await _sut.RevokeAllFamiliesForUserAsync(user.Id, "password_reset", CancellationToken.None);
        var second = await _sut.RevokeAllFamiliesForUserAsync(user.Id, "password_reset", CancellationToken.None);

        first.Should().Be(1);
        second.Should().Be(0);
    }
}
