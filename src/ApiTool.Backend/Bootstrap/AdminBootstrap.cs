using ApiTool.Backend.Auth;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Bootstrap;

/// <summary>
/// Seeds a first-boot admin user on an empty database. Runs as a startup task before
/// <c>app.Run()</c>, guarded by <c>BOOTSTRAP_ADMIN_EMAIL</c> and
/// <c>BOOTSTRAP_ADMIN_PASSWORD</c> environment variables.
/// </summary>
public sealed class AdminBootstrap(
    IServiceScopeFactory scopes,
    PasswordHasher hasher,
    ILogger<AdminBootstrap> logger,
    TimeProvider clock)
{
    /// <summary>
    /// Seeds an admin user from <paramref name="config"/>. Returns <see langword="true"/> when
    /// the user was created, <see langword="false"/> when the email already exists (idempotent).
    /// </summary>
    public async Task<bool> RunAsync(AdminBootstrapConfig config, CancellationToken ct = default)
    {
        await using var scope = scopes.CreateAsyncScope();
        var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
        var seeder = scope.ServiceProvider.GetRequiredService<TrialSeederService>();

        // Normalize email to lowercase for case-insensitive idempotency.
        var email = config.Email.ToLowerInvariant();

        var existing = await db.Users.FirstOrDefaultAsync(u => u.Email == email, ct);
        if (existing is not null)
        {
            logger.LogInformation("bootstrap: admin already exists, skipping");
            return false;
        }

        var now  = clock.GetUtcNow().UtcDateTime;
        var userId = Guid.NewGuid();
        var orgId  = Guid.NewGuid();

        // Resolve a unique org slug — use "default", appending a suffix on collision.
        var slug = await ResolveSlugAsync(db, "default", ct);

        await using var tx = await db.Database.BeginTransactionAsync(ct);
        try
        {
            var user = new User
            {
                Id           = userId,
                Email        = email,
                CreatedAt    = now,
                PasswordHash = hasher.Hash(config.Password),
                IsAdmin      = true,
            };
            db.Users.Add(user);

            var org = new Organization
            {
                Id        = orgId,
                Name      = "Default Org",
                Slug      = slug,
                OwnerId   = userId,
                Status    = OrgStatus.Active,
                CreatedAt = now,
                UpdatedAt = now,
            };
            db.Organizations.Add(org);

            db.OrganizationMembers.Add(new OrganizationMember
            {
                OrgId     = orgId,
                UserId    = userId,
                Role      = OrgRole.Owner,
                JoinedAt  = now,
            });

            db.Subscriptions.Add(new Subscription
            {
                Id                 = Guid.NewGuid(),
                OrgId              = orgId,
                Tier               = SubscriptionTier.Enterprise,
                Status             = SubscriptionStatus.Active,
                Interval           = "month",
                SeatCount          = 1,
                SeatLimit          = 1,
                CurrentPeriodStart = now,
                CurrentPeriodEnd   = now.AddYears(1),
                CreatedAt          = now,
                UpdatedAt          = now,
            });

            await db.SaveChangesAsync(ct);

            // Seed full-initial trial rows after saving the user (FK constraint requires user to exist).
            // Seeder participates in the same transaction — if CommitAsync is not called,
            // both the user and trial rows roll back together.
            // Refs docs/SPECIFICATION.md:5811 (14-day full trial at registration).
            await seeder.SeedFullInitialAsync(userId, now, ct);

            await tx.CommitAsync(ct);
        }
        catch
        {
            await tx.RollbackAsync(CancellationToken.None);
            throw;
        }

        logger.LogInformation("bootstrap: admin {Email} created (org={Slug})", email, slug);
        return true;
    }

    private static async Task<string> ResolveSlugAsync(
        AppDbContext db, string baseSlug, CancellationToken ct)
    {
        var slug = baseSlug;
        var attempt = 1;
        while (await db.Organizations.AnyAsync(o => o.Slug == slug, ct))
        {
            slug = $"{baseSlug}-{attempt++}";
        }
        return slug;
    }
}
