// Refs docs/SPECIFICATION.md:8409-8427 (github_installations lifecycle).
using System.Text.Json;
using System.Text.Json.Serialization;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.GitHub.Installations;

/// <summary>
/// A repository reference within a GitHub installation's repo_set.
/// Matches the {owner, name, id} JSON shape used in repo_set column (spec :10014).
/// </summary>
public sealed record RepoRef(
    [property: JsonPropertyName("id")] long Id,
    [property: JsonPropertyName("owner")] string Owner,
    [property: JsonPropertyName("name")] string Name);

/// <summary>
/// Business logic for GitHub installation claim/upsert and repo-set reconciliation.
/// Centralises UNIQUE-index violation translation and the two upsert paths
/// (webhook-first vs dashboard-initiated). Refs docs/SPECIFICATION.md:8409-8427.
/// </summary>
public sealed class GithubInstallationsService(
    AppDbContext db,
    TimeProvider clock,
    IInstallationTokenCache tokenCache,
    ILogger<GithubInstallationsService> log)
{
    private static readonly JsonSerializerOptions s_json =
        new(JsonSerializerDefaults.Web);

    /// <summary>
    /// Dashboard-initiated callback claim. Idempotent on (installation_id, org_id) match.
    /// If a webhook-first row exists with org_id = NULL, it is claimed for the org.
    /// Returns <see cref="InstallClaimError.AlreadyLinked"/> when the UNIQUE index rejects.
    /// </summary>
    public async Task<InstallClaimError> ClaimAsync(
        long installationId, Guid orgId, long appId, CancellationToken ct)
    {
        var existing = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);

        if (existing is not null)
        {
            if (existing.OrgId == orgId)
            {
                // Idempotent — already claimed by same org
                return InstallClaimError.None;
            }

            if (existing.OrgId is not null)
            {
                // Already claimed by a different org
                return InstallClaimError.AlreadyLinked;
            }

            // Webhook-first row: org_id = NULL — claim it
            existing.OrgId = orgId;
            existing.ClaimedAt = clock.GetUtcNow().UtcDateTime;

            try
            {
                await db.SaveChangesAsync(ct);
                return InstallClaimError.None;
            }
            catch (DbUpdateException ex) when (IsUniqueIndexViolation(ex))
            {
                log.LogWarning(
                    "install_already_linked installation_id={Id} org_id={OrgId}", installationId, orgId);
                return InstallClaimError.AlreadyLinked;
            }
        }

        // Insert a new row
        var row = new GithubInstallation
        {
            InstallationId = installationId,
            AppId = appId,
            OrgId = orgId,
            AccountLogin = string.Empty, // Populated when webhook arrives
            AccountType = "Organization",
            RepoSelection = "selected",
            RepoSetJson = "[]",
            InstalledAt = clock.GetUtcNow().UtcDateTime,
            ClaimedAt = clock.GetUtcNow().UtcDateTime,
            LastReconciledAt = clock.GetUtcNow().UtcDateTime,
        };
        db.GithubInstallations.Add(row);

        try
        {
            await db.SaveChangesAsync(ct);
            return InstallClaimError.None;
        }
        catch (DbUpdateException ex) when (IsUniqueIndexViolation(ex))
        {
            log.LogWarning(
                "install_already_linked installation_id={Id} org_id={OrgId}", installationId, orgId);
            return InstallClaimError.AlreadyLinked;
        }
    }

    /// <summary>
    /// Handles <c>installation.created</c> webhook — upserts with org_id = NULL when no claim is pending.
    /// Idempotent on repeated delivery (spec :8416-8417).
    /// </summary>
    public async Task UpsertFromWebhookAsync(
        long installationId, long appId, string accountLogin, string accountType,
        string repoSelection, IReadOnlyList<RepoRef> initialRepos, CancellationToken ct)
    {
        var existing = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId, ct);

        if (existing is not null)
        {
            // Idempotent — row already exists; preserve any prior org claim
            log.LogDebug(
                "installation.created duplicate delivery installation_id={Id}", installationId);
            return;
        }

        var repoSetJson = JsonSerializer.Serialize(initialRepos, s_json);

        db.GithubInstallations.Add(new GithubInstallation
        {
            InstallationId = installationId,
            AppId = appId,
            OrgId = null,
            AccountLogin = accountLogin,
            AccountType = accountType,
            RepoSelection = repoSelection,
            RepoSetJson = repoSetJson,
            InstalledAt = clock.GetUtcNow().UtcDateTime,
            LastReconciledAt = clock.GetUtcNow().UtcDateTime,
        });

        await db.SaveChangesAsync(ct);

        log.LogInformation(
            "github_installation_created installation_id={Id} account={Login}", installationId, accountLogin);
    }

    /// <summary>
    /// Handles <c>installation_repositories.added</c> — set-union, idempotent (spec :8419).
    /// </summary>
    public async Task ApplyRepoAddedAsync(
        long installationId, IReadOnlyList<RepoRef> repos, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("ApplyRepoAdded: installation {Id} not found", installationId);
            return;
        }

        var current = JsonSerializer.Deserialize<List<RepoRef>>(row.RepoSetJson, s_json) ?? [];
        var ids = new HashSet<long>(current.Select(r => r.Id));

        foreach (var repo in repos)
        {
            if (ids.Add(repo.Id))
                current.Add(repo);
        }

        row.RepoSetJson = JsonSerializer.Serialize(current, s_json);
        await db.SaveChangesAsync(ct);
    }

    /// <summary>
    /// Replaces <c>repo_set</c> with the authoritative list from the GitHub API,
    /// computing additions and removals. Calls <see cref="ApplyRepoRemovedAsync"/>
    /// for removed repos so in-flight pr_checks are marked appropriately.
    /// Used by the daily reconciler (spec :8425).
    /// </summary>
    public async Task ReplaceRepoSetAsync(
        long installationId, IReadOnlyList<RepoRef> remoteRepos, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("ReplaceRepoSet: installation {Id} not found", installationId);
            return;
        }

        var current = JsonSerializer.Deserialize<List<RepoRef>>(row.RepoSetJson, s_json) ?? [];
        var remoteIds = new HashSet<long>(remoteRepos.Select(r => r.Id));
        var currentIds = new HashSet<long>(current.Select(r => r.Id));

        // Repos present locally but absent from GitHub → removed
        var removals = current.Where(r => !remoteIds.Contains(r.Id)).ToList();

        // Repos present on GitHub but absent locally → added
        var additions = remoteRepos.Where(r => !currentIds.Contains(r.Id)).ToList();

        if (removals.Count > 0)
            await ApplyRepoRemovedAsync(installationId, removals, ct);

        if (additions.Count > 0)
            await ApplyRepoAddedAsync(installationId, additions, ct);
    }

    /// <summary>
    /// Handles <c>installation_repositories.removed</c> — set-difference; marks in-flight
    /// pr_checks for removed repos as <c>REPO_NOT_COVERED</c> (spec :8420, ≤20 chars).
    /// </summary>
    public async Task ApplyRepoRemovedAsync(
        long installationId, IReadOnlyList<RepoRef> repos, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("ApplyRepoRemoved: installation {Id} not found", installationId);
            return;
        }

        var removedSlugs = repos.Select(r => $"{r.Owner}/{r.Name}").ToHashSet(StringComparer.OrdinalIgnoreCase);

        var current = JsonSerializer.Deserialize<List<RepoRef>>(row.RepoSetJson, s_json) ?? [];
        var removedIds = repos.Select(r => r.Id).ToHashSet();
        current.RemoveAll(r => removedIds.Contains(r.Id));
        row.RepoSetJson = JsonSerializer.Serialize(current, s_json);

        // Mark in-flight pr_checks for removed repos
        if (removedSlugs.Count > 0 && row.OrgId.HasValue)
        {
            var affected = await db.PrChecks
                .Where(p => p.OrgId == row.OrgId.Value && removedSlugs.Contains(p.Repo))
                .ToListAsync(ct);

            foreach (var check in affected)
                check.State = "REPO_NOT_COVERED"; // ≤20 chars (pr_checks.State MaxLength)
        }

        await db.SaveChangesAsync(ct);

        log.LogInformation(
            "github_repos_removed installation_id={Id} count={Count}", installationId, repos.Count);
    }

    // ── Lifecycle handlers (M14-019) ──────────────────────────────────────────

    /// <summary>
    /// Handles <c>installation.suspend</c> — sets <c>suspended_at</c> and evicts the M14-018 token cache.
    /// Spec :8561.
    /// </summary>
    public async Task MarkSuspendedAsync(long installationId, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("MarkSuspended: installation {Id} not found", installationId);
            return;
        }
        row.SuspendedAt = clock.GetUtcNow().UtcDateTime;
        await db.SaveChangesAsync(ct);
        tokenCache.Evict(installationId);
        log.LogInformation("github_installation_suspended installation_id={Id}", installationId);
    }

    /// <summary>
    /// Handles <c>installation.unsuspend</c> — clears <c>suspended_at</c>. Spec :8562.
    /// </summary>
    public async Task MarkUnsuspendedAsync(long installationId, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("MarkUnsuspended: installation {Id} not found", installationId);
            return;
        }
        row.SuspendedAt = null;
        await db.SaveChangesAsync(ct);
        log.LogInformation("github_installation_unsuspended installation_id={Id}", installationId);
    }

    /// <summary>
    /// Handles <c>installation.deleted</c> — soft-deletes the installation, evicts the token cache,
    /// and marks all open <c>pr_checks</c> for the org as <c>INSTALLATION_DELETED</c>. Spec :8560.
    /// </summary>
    public async Task MarkDeletedAsync(long installationId, CancellationToken ct)
    {
        var row = await db.GithubInstallations
            .FirstOrDefaultAsync(x => x.InstallationId == installationId && x.DeletedAt == null, ct);
        if (row is null)
        {
            log.LogWarning("MarkDeleted: installation {Id} not found", installationId);
            return;
        }
        row.DeletedAt = clock.GetUtcNow().UtcDateTime;

        if (row.OrgId.HasValue)
        {
            var openChecks = await db.PrChecks
                .Where(p => p.OrgId == row.OrgId.Value
                    && p.Status != "posted" && p.Status != "failed")
                .ToListAsync(ct);
            foreach (var check in openChecks)
                check.State = "INSTALLATION_DELETED"; // 20 chars exactly (pr_checks.State MaxLength)
        }

        await db.SaveChangesAsync(ct);
        tokenCache.Evict(installationId);
        log.LogInformation("github_installation_deleted installation_id={Id}", installationId);
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    private static bool IsUniqueIndexViolation(DbUpdateException ex)
    {
        // SQLite error code 19 (SQLITE_CONSTRAINT); check both the index name and column name
        // because SQLite may report either form depending on the SQLite version.
        if (ex.InnerException is SqliteException sqEx && sqEx.SqliteErrorCode == 19)
            return sqEx.Message.Contains("idx_github_installations_org",
                       StringComparison.OrdinalIgnoreCase)
                || sqEx.Message.Contains("github_installations.org_id",
                       StringComparison.OrdinalIgnoreCase);

        // Npgsql (Postgres): SqlState 23505 — use reflection to avoid a compile-time Npgsql dependency
        // while still retaining type safety (no dynamic dispatch, no RuntimeBinderException risk).
        if (ex.InnerException?.GetType().Name == "PostgresException")
        {
            var sqlState = ex.InnerException.GetType()
                .GetProperty("SqlState")
                ?.GetValue(ex.InnerException) as string;
            return sqlState == "23505";
        }

        return false;
    }
}
