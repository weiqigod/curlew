// Dev/Testing-only endpoint for seeding M14-021 convergence test state.
// NEVER registered in Production. Guarded by InternalAccessFilter as a second layer.
// Used by scripts/seed-test-data.sh and the M14-021 Playwright spec.
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Internal;

/// <summary>
/// Maps <c>POST /internal/test/seed-m14</c> (Development + Testing only).
/// Idempotently upgrades the given org to team tier, marks its fixture owner as
/// email-verified, ensures a Stripe customer mirror row, and upserts a
/// <c>github_installations</c> row pointing at the test GitHub App sidecar.
/// </summary>
public static class InternalSeedM14Endpoint
{
    /// <summary>
    /// Conditionally registers the seed-m14 endpoint when the environment is Development or Testing.
    /// </summary>
    public static IEndpointRouteBuilder MapInternalSeedM14Endpoint(
        this IEndpointRouteBuilder app, IWebHostEnvironment env)
    {
        if (!env.IsDevelopment() && !env.IsEnvironment("Testing"))
            return app;

        app.MapPost("/internal/test/seed-m14", async (
            SeedM14Request? body,
            AppDbContext db,
            TimeProvider clock,
            CancellationToken ct) =>
        {
            if (body is null)
                return HttpResults.BadRequest("request body is required");

            if (body.OrgId == Guid.Empty)
                return HttpResults.BadRequest("org_id is required");

            var now = clock.GetUtcNow().UtcDateTime;
            var requestedTier = SubscriptionTier.Team;
            if (!string.IsNullOrWhiteSpace(body.Tier)
                && (!Enum.TryParse(body.Tier, ignoreCase: true, out requestedTier)
                    || requestedTier is not (SubscriptionTier.Team or SubscriptionTier.Enterprise)))
            {
                return HttpResults.BadRequest("tier must be team or enterprise");
            }

            // 1. Resolve the fixture organization and owner.
            var ownerId = await db.Organizations
                .Where(o => o.Id == body.OrgId)
                .Select(o => (Guid?)o.OwnerId)
                .FirstOrDefaultAsync(ct);
            if (ownerId is null)
                return HttpResults.NotFound("organization owner not found for the given org_id");
            var owner = await db.Users.FirstOrDefaultAsync(u => u.Id == ownerId.Value, ct);
            if (owner is null)
                return HttpResults.NotFound("organization owner not found for the given org_id");

            // 2. Ensure an active Team subscription. Newly created fixture
            // organizations do not have a subscription row yet.
            var sub = await db.Subscriptions.FirstOrDefaultAsync(s => s.OrgId == body.OrgId, ct);
            if (sub is null)
            {
                sub = new Subscription
                {
                    Id = Guid.NewGuid(),
                    OrgId = body.OrgId,
                    Tier = requestedTier,
                    Status = SubscriptionStatus.Active,
                    SeatCount = 1,
                    SeatLimit = requestedTier == SubscriptionTier.Enterprise ? 5 : 3,
                    CurrentPeriodStart = now,
                    CurrentPeriodEnd = now.AddMonths(1),
                    CreatedAt = now,
                    UpdatedAt = now,
                };
                db.Subscriptions.Add(sub);
            }
            else
            {
                // Seed calls may run in different milestone workflows. Never
                // let an older Team fixture downgrade an Enterprise org.
                if (requestedTier > sub.Tier)
                    sub.Tier = requestedTier;
                if (requestedTier == SubscriptionTier.Enterprise && sub.SeatLimit < 5)
                    sub.SeatLimit = 5;
                sub.UpdatedAt = now;
            }
            if (!string.IsNullOrEmpty(body.StripeCustomerId))
                sub.StripeCustomerId = body.StripeCustomerId;

            // Checkout and other mutation surfaces require verified email. The
            // test-token flow has no browser verification round-trip, so make
            // the seeded organization owner an explicit verified fixture.
            owner.EmailVerified = true;

            if (body.VerifiedUserId is { } verifiedUserId)
            {
                var verifiedUser = await db.Users.FirstOrDefaultAsync(u => u.Id == verifiedUserId, ct);
                if (verifiedUser is null)
                {
                    if (string.IsNullOrWhiteSpace(body.VerifiedUserEmail))
                        return HttpResults.BadRequest("verified_user_email is required with verified_user_id");
                    verifiedUser = new User
                    {
                        Id = verifiedUserId,
                        Email = body.VerifiedUserEmail,
                        EmailVerified = true,
                        CreatedAt = now,
                    };
                    db.Users.Add(verifiedUser);
                }
                else
                {
                    verifiedUser.EmailVerified = true;
                }
            }

            // 3. Insert/update github_installations row. The schema allows one
            // installation per organization, so re-seeding with a different
            // fixture installation id must update the existing row rather than
            // violate the unique org_id index.
            var existing = await db.GithubInstallations
                .FirstOrDefaultAsync(gi => gi.OrgId == body.OrgId, ct);

            if (existing is null)
            {
                var repoSetJson = SerializeRepoSet(body.Repos);

                db.GithubInstallations.Add(new GithubInstallation
                {
                    InstallationId = body.InstallationId,
                    AppId = body.AppId,
                    OrgId = body.OrgId,
                    AccountLogin = "test-installation",
                    AccountType = "Organization",
                    RepoSelection = "selected",
                    RepoSetJson = repoSetJson,
                    InstalledAt = now,
                    ClaimedAt = now,
                    LastReconciledAt = now,
                });
            }
            else
            {
                existing.AppId = body.AppId;
                existing.RepoSetJson = SerializeRepoSet(body.Repos);
                existing.ClaimedAt = now;
                existing.LastReconciledAt = now;
            }

            await db.SaveChangesAsync(ct);

            var responseBody = new
            {
                subscription_status = sub.Status.ToString().ToLowerInvariant(),
                subscription_tier = sub.Tier.ToString().ToLowerInvariant(),
                installation_id = existing?.InstallationId ?? body.InstallationId,
            };
            return HttpResults.Ok(responseBody);
        })
        .AllowAnonymous()
        .DisableAntiforgery()
        .AddEndpointFilter<InternalAccessFilter>()
        .WithName("SeedM14")
        .WithTags("Internal");

        return app;
    }

    /// <summary>
    /// Serialises the seeded repository slugs into the shape readers expect:
    /// an array of objects carrying <c>full_name</c>. CheckRunPoster.IsRepoCovered
    /// walks the entries with <c>TryGetProperty</c>, so a bare string array made it
    /// throw — leaving every seeded pr-check silently stuck in "queued" instead of
    /// posting a check run.
    /// </summary>
    private static string SerializeRepoSet(string[]? repos) =>
        JsonSerializer.Serialize((repos ?? []).Select(r => new { full_name = r }));
}

/// <summary>Request body for the M14 seed endpoint.</summary>
internal sealed record SeedM14Request(
    Guid OrgId,
    long InstallationId,
    long AppId,
    string[]? Repos,
    string? StripeCustomerId,
    string? Tier,
    Guid? VerifiedUserId,
    string? VerifiedUserEmail);
