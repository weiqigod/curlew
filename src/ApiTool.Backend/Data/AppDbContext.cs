using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.EntityFrameworkCore;

namespace ApiTool.Backend.Data;

/// <summary>EF Core database context for the ApiTool backend.</summary>
public sealed class AppDbContext(DbContextOptions<AppDbContext> options) : DbContext(options)
{
    /// <summary>Users table.</summary>
    public DbSet<User> Users => Set<User>();

    /// <summary>Organizations table.</summary>
    public DbSet<Organization> Organizations => Set<Organization>();

    /// <summary>Organization membership table.</summary>
    public DbSet<OrganizationMember> OrganizationMembers => Set<OrganizationMember>();

    /// <summary>Organization invitations table.</summary>
    public DbSet<OrganizationInvitation> OrganizationInvitations => Set<OrganizationInvitation>();

    /// <summary>Organization billing subscriptions.</summary>
    public DbSet<Subscription> Subscriptions => Set<Subscription>();

    /// <summary>Organization audit log table.</summary>
    public DbSet<OrganizationAuditLogEntry> OrganizationAuditLog => Set<OrganizationAuditLogEntry>();

    /// <summary>Uploaded test run headers.</summary>
    public DbSet<Result> Results => Set<Result>();

    /// <summary>Per-test rows for uploaded runs.</summary>
    public DbSet<ResultItem> ResultItems => Set<ResultItem>();

    /// <summary>Cron schedules table.</summary>
    public DbSet<Schedule> Schedules => Set<Schedule>();

    /// <summary>Enqueued/completed run instances from schedules.</summary>
    public DbSet<ScheduledRun> ScheduledRuns => Set<ScheduledRun>();

    /// <summary>Notification rules for an organization.</summary>
    public DbSet<NotificationRule> NotificationRules => Set<NotificationRule>();

    /// <summary>Delivery attempts for notification rules.</summary>
    public DbSet<NotificationDelivery> NotificationDeliveries => Set<NotificationDelivery>();

    /// <summary>PR check rows posted by the CLI.</summary>
    public DbSet<PrCheck> PrChecks => Set<PrCheck>();

    /// <summary>IdP signing certificates for SAML SSO.</summary>
    public DbSet<SsoCredential> SsoCredentials => Set<SsoCredential>();

    /// <summary>Custom organization roles.</summary>
    public DbSet<CustomRole> OrganizationCustomRoles => Set<CustomRole>();

    /// <summary>Distributed execution jobs.</summary>
    public DbSet<CoordinatorJob> CoordinatorJobs => Set<CoordinatorJob>();

    /// <summary>Shards belonging to a distributed execution job.</summary>
    public DbSet<CoordinatorShard> CoordinatorShards => Set<CoordinatorShard>();

    /// <summary>JWT signing-key registry (one row per kid).</summary>
    public DbSet<SigningKey> SigningKeys => Set<SigningKey>();

    /// <summary>Opaque refresh tokens with family-revocation rotation tracking.</summary>
    public DbSet<RefreshToken> RefreshTokens => Set<RefreshToken>();

    /// <summary>Stripe webhook event deliveries — idempotency store and audit trail.</summary>
    public DbSet<StripeWebhookEvent> StripeWebhookEvents => Set<StripeWebhookEvent>();

    /// <summary>Stripe invoices mirrored by invoice webhook handlers (M14-013).</summary>
    public DbSet<Invoice> Invoices => Set<Invoice>();

    /// <summary>Stripe payment methods attached to org customers, soft-deleted on detach (M14-013).</summary>
    public DbSet<PaymentMethod> PaymentMethods => Set<PaymentMethod>();

    /// <summary>GitHub App installation registry — cross-tenant guardrail (M14-017).</summary>
    public DbSet<GithubInstallation> GithubInstallations => Set<GithubInstallation>();

    /// <summary>GitHub webhook event deliveries — idempotency store and audit trail (M14-019).</summary>
    public DbSet<GithubWebhookEvent> GithubWebhookEvents => Set<GithubWebhookEvent>();

    /// <summary>Password-reset tokens (hash-stored, 30-min lifetime, single-use). Refs spec :8479-8493.</summary>
    public DbSet<PasswordResetToken> PasswordResetTokens => Set<PasswordResetToken>();

    /// <summary>Email-verification tokens (hash-stored, 24h lifetime, single-use). Refs spec :8495-8509.</summary>
    public DbSet<EmailVerificationToken> EmailVerificationTokens => Set<EmailVerificationToken>();

    /// <summary>GitLab App installation registry with PAT-based auth (M16-013).</summary>
    public DbSet<GitLabInstallation> GitLabInstallations => Set<GitLabInstallation>();

    /// <summary>GitLab webhook event deliveries — idempotency store (M16-013).</summary>
    public DbSet<GitLabWebhookEvent> GitLabWebhookEvents => Set<GitLabWebhookEvent>();

    /// <summary>Per-(user, feature) trial grants — single-use per feature, ever (M16-005).</summary>
    public DbSet<Trial> Trials => Set<Trial>();

    /// <summary>Shared vault configuration templates — one row per Team-tier org (M16-017).</summary>
    public DbSet<TeamVault> TeamVaults => Set<TeamVault>();

    /// <summary>Per-user GDPR data-export requests (M18-004).</summary>
    public DbSet<UserExportRequest> UserExportRequests => Set<UserExportRequest>();

    /// <summary>Hash-stored, single-use, 5-minute re-auth tokens gating deletion requests (M18-005).</summary>
    public DbSet<DeletionReauthToken> DeletionReauthTokens => Set<DeletionReauthToken>();

    /// <summary>Raw telemetry events emitted by CLI installations (M18-007). Purged after 90 days.</summary>
    public DbSet<TelemetryEvent> TelemetryEvents => Set<TelemetryEvent>();

    /// <summary>Daily per-(event_type, day) aggregates produced by TelemetryAggregatorHost (M18-007). Persistent.</summary>
    public DbSet<TelemetryDailyAggregate> TelemetryDailyAggregates => Set<TelemetryDailyAggregate>();

    /// <inheritdoc/>
    protected override void OnModelCreating(ModelBuilder b)
    {
        b.Entity<User>(e =>
        {
            e.ToTable("users");
            e.HasKey(x => x.Id);
            e.Property(x => x.Email).HasMaxLength(255).IsRequired();
            e.HasIndex(x => x.Email).IsUnique();
            e.Property(x => x.PasswordHash).HasColumnName("password_hash").HasMaxLength(200);
            e.Property(x => x.IsAdmin).HasColumnName("is_admin").HasDefaultValue(false);
            e.Property(x => x.EmailVerified).HasColumnName("email_verified").HasDefaultValue(false);
            e.Property(x => x.PendingDeletionAt).HasColumnName("pending_deletion_at");
            e.Property(x => x.AnonymisedAt).HasColumnName("anonymised_at");
        });

        b.Entity<Organization>(e =>
        {
            e.ToTable("organizations");
            e.HasKey(x => x.Id);
            e.Property(x => x.Name).HasMaxLength(100).IsRequired();
            e.Property(x => x.Slug).HasMaxLength(100).IsRequired();
            e.HasIndex(x => x.Slug).IsUnique();
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.SettingsJson).HasColumnName("settings").IsRequired();
            e.Property(x => x.AuditLogRetentionDays)
                .HasColumnName("audit_log_retention_days")
                .HasDefaultValue(365)
                .IsRequired();
            e.HasOne<User>().WithMany().HasForeignKey(x => x.OwnerId).OnDelete(DeleteBehavior.Restrict);
        });

        b.Entity<OrganizationMember>(e =>
        {
            e.ToTable("organization_members");
            e.HasKey(x => new { x.OrgId, x.UserId });
            e.Property(x => x.Role).HasConversion<string>().HasMaxLength(20);
            e.HasIndex(x => new { x.OrgId, x.Role });
            e.Property(x => x.PermissionsJson).HasColumnName("permissions").IsRequired();
            e.Property(x => x.RoleId).HasColumnName("role_id").IsRequired(false);
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<CustomRole>(e =>
        {
            e.ToTable("organization_custom_roles");
            e.HasKey(x => x.Id);
            e.Property(x => x.Name).HasMaxLength(100).IsRequired();
            e.Property(x => x.PermissionsJson).HasColumnName("permissions").IsRequired();
            e.Property(x => x.CreatedBy).IsRequired(false);
            e.HasIndex(x => new { x.OrgId, x.Name }).IsUnique();
            e.HasIndex(x => x.CreatedBy);
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
        });

        b.Entity<OrganizationInvitation>(e =>
        {
            e.ToTable("organization_invitations");
            e.HasKey(x => x.Id);
            e.Property(x => x.Email).HasMaxLength(255).IsRequired();
            e.Property(x => x.EmailNormalized).HasMaxLength(255).IsRequired();
            e.Property(x => x.TokenHash).HasColumnName("token_hash").HasMaxLength(64).IsRequired();
            e.Property(x => x.Role).HasConversion<string>().HasMaxLength(20);
            e.HasIndex(x => x.TokenHash).IsUnique();
            e.HasIndex(x => new { x.OrgId, x.EmailNormalized, x.AcceptedAt, x.RevokedAt });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<Subscription>(e =>
        {
            e.ToTable("subscriptions");
            e.HasKey(x => x.Id);
            e.Property(x => x.Tier).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.Interval).HasMaxLength(10).IsRequired();
            e.Property(x => x.StripeCustomerId).HasMaxLength(100);
            e.Property(x => x.StripeSubscriptionId).HasMaxLength(100);
            e.Property(x => x.StripeCustomerEmail).HasMaxLength(320);
            e.HasIndex(x => x.OrgId).IsUnique();
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<OrganizationAuditLogEntry>(e =>
        {
            e.ToTable("organization_audit_log");
            e.HasKey(x => x.Id);
            e.Property(x => x.OrgId).IsRequired(false);
            e.Property(x => x.ActorId).HasColumnName("actor_id").IsRequired(false);
            e.Property(x => x.EventType).HasMaxLength(100).IsRequired();
            e.Property(x => x.PayloadJson).HasColumnName("payload").IsRequired();
            e.Property(x => x.TargetType).HasColumnName("target_type").HasMaxLength(50);
            e.Property(x => x.TargetId).HasColumnName("target_id");
            e.Property(x => x.PreviousStateJson).HasColumnName("previous_state");
            e.Property(x => x.NewStateJson).HasColumnName("new_state");
            e.Property(x => x.IpAddress).HasColumnName("ip_address").HasMaxLength(45);
            e.Property(x => x.UserAgent).HasColumnName("user_agent").HasMaxLength(500);
            e.Property(x => x.Success).HasColumnName("success").IsRequired();
            e.Property(x => x.FailureReason).HasColumnName("failure_reason").HasMaxLength(200);
            e.Property(x => x.ActorEmail).HasColumnName("actor_email").HasMaxLength(320);
            e.HasIndex(x => new { x.OrgId, x.CreatedAt });
            e.HasIndex(x => x.EventType);
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<Result>(e =>
        {
            e.ToTable("results");
            e.HasKey(x => x.Id);
            e.Property(x => x.CollectionName).HasMaxLength(200).IsRequired();
            e.Property(x => x.TriggeredBy).HasMaxLength(50);
            e.Property(x => x.GitSha).HasMaxLength(64);
            e.HasIndex(x => new { x.OrgId, x.CreatedAt });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UploadedBy).OnDelete(DeleteBehavior.Restrict);
        });

        b.Entity<ResultItem>(e =>
        {
            e.ToTable("result_items");
            e.HasKey(x => x.Id);
            e.Property(x => x.Name).HasMaxLength(500).IsRequired();
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.Message).HasMaxLength(4000);
            // M16-019: method/request_url/path_template for failures aggregation
            e.Property(x => x.Method).HasColumnName("method").HasMaxLength(10);
            e.Property(x => x.RequestUrl).HasColumnName("request_url").HasMaxLength(1000);
            e.Property(x => x.PathTemplate).HasColumnName("path_template").HasMaxLength(500);
            e.HasIndex(x => new { x.ResultId, x.Ordinal });
            // Supports the failures aggregation grouped by (method, path_template) restricted to Status = Failed.
            e.HasIndex(x => new { x.Status, x.Method, x.PathTemplate })
                .HasDatabaseName("IX_result_items_Status_Method_PathTemplate");
            e.HasOne<Result>().WithMany().HasForeignKey(x => x.ResultId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<Schedule>(e =>
        {
            e.ToTable("schedules");
            e.HasKey(x => x.Id);
            e.Property(x => x.Name).HasMaxLength(100).IsRequired();
            e.Property(x => x.CronExpression).HasMaxLength(100).IsRequired();
            e.Property(x => x.Timezone).HasMaxLength(64).IsRequired().HasDefaultValue("UTC");
            e.Property(x => x.CollectionRef).HasMaxLength(500).IsRequired();
            e.Property(x => x.CreatedBy).IsRequired(false);
            e.HasIndex(x => new { x.OrgId, x.Name }).IsUnique();
            e.HasIndex(x => new { x.Enabled, x.NextRunAt });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
            // M18-009: envelope-encrypted env_vars columns (new, no plaintext predecessor)
            if (Database.IsNpgsql())
                e.Property(x => x.EnvVarsCiphertext).HasColumnName("env_vars_ciphertext").HasColumnType("bytea").IsRequired(false);
            else
                e.Property(x => x.EnvVarsCiphertext).HasColumnName("env_vars_ciphertext").IsRequired(false);
            e.Property(x => x.EnvVarsKid).HasColumnName("env_vars_kid").HasMaxLength(500).IsRequired(false);
        });

        b.Entity<ScheduledRun>(e =>
        {
            e.ToTable("scheduled_runs");
            e.HasKey(x => x.Id);
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.ClaimedByWorker).HasMaxLength(200);
            e.Property(x => x.FailureReason).HasMaxLength(2000);
            e.HasIndex(x => new { x.ScheduleId, x.CreatedAt });
            // M16-009: reaper scans status='running' AND last_heartbeat_at < cutoff
            e.HasIndex(x => x.LastHeartbeatAt).HasDatabaseName("IX_scheduled_runs_LastHeartbeatAt");
            // M16-009: next-run scans by status='queued'; composite covers order by created_at
            e.HasIndex(x => new { x.Status, x.CreatedAt }).HasDatabaseName("IX_scheduled_runs_Status_CreatedAt");
            e.HasOne<Schedule>().WithMany().HasForeignKey(x => x.ScheduleId).OnDelete(DeleteBehavior.Cascade);
            // M16-009: optional FK to result row (set when worker posts result)
            e.HasOne<Result>().WithMany().HasForeignKey(x => x.ResultId).OnDelete(DeleteBehavior.SetNull).IsRequired(false);
        });

        b.Entity<NotificationRule>(e =>
        {
            e.ToTable("notification_rules");
            e.HasKey(x => x.Id);
            e.Property(x => x.Channel).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.Target).HasMaxLength(500).IsRequired();
            e.Property(x => x.OnEvents).HasMaxLength(200).IsRequired();
            e.Property(x => x.CreatedBy).IsRequired(false);
            e.HasIndex(x => new { x.OrgId, x.Channel });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
        });

        b.Entity<NotificationDelivery>(e =>
        {
            e.ToTable("notification_deliveries");
            e.HasKey(x => x.Id);
            e.Property(x => x.Channel).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.ErrorMessage).HasMaxLength(1000);
            e.HasIndex(x => new { x.OrgId, x.AttemptedAt });
            e.HasOne<NotificationRule>().WithMany().HasForeignKey(x => x.RuleId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<PrCheck>(e =>
        {
            e.ToTable("pr_checks");
            e.HasKey(x => x.Id);
            e.Property(x => x.Repo).HasMaxLength(200).IsRequired();
            e.Property(x => x.State).HasMaxLength(20).IsRequired();
            // M14-018 additions
            e.Property(x => x.Status).HasColumnName("status").HasMaxLength(30).IsRequired().HasDefaultValue("pending");
            e.Property(x => x.Conclusion).HasColumnName("conclusion").HasMaxLength(20);
            e.Property(x => x.DetailsUrl).HasColumnName("details_url").HasMaxLength(500);
            e.Property(x => x.LastError).HasColumnName("last_error").HasMaxLength(500);
            e.Property(x => x.HeadSha).HasColumnName("head_sha").HasMaxLength(64).IsRequired().HasDefaultValue(string.Empty);
            e.Property(x => x.OutputSummary).HasColumnName("output_summary");
            e.Property(x => x.OutputText).HasColumnName("output_text");
            e.Property(x => x.InstallationId).HasColumnName("installation_id");
            e.Property(x => x.CheckRunId).HasColumnName("check_run_id");
            e.Property(x => x.ExternalId).HasColumnName("external_id").IsRequired();
            e.Property(x => x.PostingStartedAt).HasColumnName("posting_started_at");
            e.Property(x => x.PostedAt).HasColumnName("posted_at");
            e.Property(x => x.AttemptCount).HasColumnName("attempt_count").HasDefaultValue(0);
            if (Database.IsNpgsql())
                e.Property(x => x.AnnotationsJson).HasColumnType("jsonb").HasColumnName("annotations");
            else
                e.Property(x => x.AnnotationsJson).HasColumnName("annotations");
            e.HasIndex(x => new { x.OrgId, x.CreatedAt });
            e.HasIndex(x => x.ExternalId).IsUnique().HasDatabaseName("idx_pr_checks_external_id");
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            // M16-014 additions: provider discriminator + GitLab-specific columns
            e.Property(x => x.Provider).HasColumnName("provider").HasMaxLength(20).IsRequired().HasDefaultValue("github");
            e.Property(x => x.GitLabInstallationId).HasColumnName("gitlab_installation_id");
            e.Property(x => x.GitLabStatusId).HasColumnName("gitlab_status_id");
            e.ToTable(t => t.HasCheckConstraint(
                "ck_pr_checks_provider",
                "\"provider\" IN ('github', 'gitlab')"));
            e.HasOne<GitLabInstallation>()
                .WithMany()
                .HasForeignKey(x => x.GitLabInstallationId)
                .OnDelete(DeleteBehavior.SetNull);
            e.HasIndex(x => x.Provider).HasDatabaseName("idx_pr_checks_provider");
        });

        b.Entity<SsoCredential>(e =>
        {
            e.ToTable("sso_credentials");
            e.HasKey(x => x.Id);
            e.Property(x => x.Kid).HasMaxLength(100).IsRequired();
            e.Property(x => x.PublicCertPem).IsRequired();
            e.HasIndex(x => new { x.OrgId, x.Kid }).IsUnique();
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<CoordinatorJob>(e =>
        {
            e.ToTable("coordinator_jobs");
            e.HasKey(x => x.Id);
            e.Property(x => x.CollectionSha).HasColumnName("collection_sha").HasMaxLength(128).IsRequired();
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.AggregateResultId).HasColumnName("aggregate_result_id");
            e.Property(x => x.CreatedBy).IsRequired(false);
            e.HasIndex(x => new { x.OrgId, x.CreatedAt });
            e.HasIndex(x => x.Status);
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
        });

        b.Entity<CoordinatorShard>(e =>
        {
            e.ToTable("coordinator_shards");
            e.HasKey(x => x.Id);
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20);
            e.Property(x => x.AssignedWorker).HasColumnName("assigned_worker").HasMaxLength(100);
            e.Property(x => x.RequestsJson).HasColumnName("requests").IsRequired();
            e.Property(x => x.ResultJson).HasColumnName("result");
            e.HasIndex(x => new { x.JobId, x.ShardIndex }).IsUnique();
            e.HasIndex(x => new { x.Status, x.LastHeartbeatAt });
            e.HasOne<CoordinatorJob>().WithMany().HasForeignKey(x => x.JobId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<SigningKey>(e =>
        {
            e.ToTable("signing_keys");
            e.HasKey(x => x.Kid);
            e.Property(x => x.Kid).HasColumnName("kid").HasMaxLength(64).IsRequired();
            e.Property(x => x.Algorithm).HasColumnName("algorithm").HasMaxLength(20).IsRequired();
            e.Property(x => x.Status).HasColumnName("status").HasMaxLength(20).IsRequired();
            e.Property(x => x.KmsKeyId).HasColumnName("kms_key_id");
            e.Property(x => x.PublicKeyJwkJson).HasColumnName("public_key_jwk").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.PublicKeyJwkJson).HasColumnType("jsonb");
            e.Property(x => x.CreatedAt).HasColumnName("created_at").IsRequired();
            e.Property(x => x.PromotedAt).HasColumnName("promoted_at");
            e.Property(x => x.RetiringAt).HasColumnName("retiring_at");
            e.Property(x => x.RevokedAt).HasColumnName("revoked_at");
            e.Property(x => x.RevokeReason).HasColumnName("revoke_reason");
            // Partial unique indexes enforce at most one key per status for 'current' and 'next'.
            e.HasIndex(x => x.Status)
                .HasDatabaseName("idx_signing_keys_current")
                .HasFilter("\"status\" = 'current'")
                .IsUnique();
            e.HasIndex(x => x.Status)
                .HasDatabaseName("idx_signing_keys_next")
                .HasFilter("\"status\" = 'next'")
                .IsUnique();
        });

        b.Entity<RefreshToken>(e =>
        {
            e.ToTable("refresh_tokens");
            e.HasKey(x => x.Id);
            e.Property(x => x.TokenHash).HasColumnName("token_hash").IsRequired();
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            // device_id is a plain column here; FK to devices(id) added in M14-002.
            e.Property(x => x.DeviceId).HasColumnName("device_id").IsRequired();
            e.Property(x => x.FamilyId).HasColumnName("family_id").IsRequired();
            e.Property(x => x.ParentId).HasColumnName("parent_id");
            e.Property(x => x.IssuedAt).HasColumnName("issued_at").IsRequired();
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at").IsRequired();
            e.Property(x => x.RotatedAt).HasColumnName("rotated_at");
            e.Property(x => x.RevokedAt).HasColumnName("revoked_at");
            e.Property(x => x.RevokeReason).HasColumnName("revoke_reason");
            e.Property(x => x.LastUsedIp).HasColumnName("last_used_ip");
            e.Property(x => x.UserAgent).HasColumnName("user_agent");
            e.HasIndex(x => x.TokenHash).IsUnique();
            e.HasIndex(x => x.FamilyId).HasDatabaseName("idx_refresh_tokens_family");
            e.HasIndex(x => new { x.UserId, x.DeviceId }).HasDatabaseName("idx_refresh_tokens_user_device");
            e.HasOne<RefreshToken>().WithMany().HasForeignKey(x => x.ParentId).OnDelete(DeleteBehavior.SetNull);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<Invoice>(e =>
        {
            e.ToTable("invoices");
            e.HasKey(x => x.Id);
            e.Property(x => x.StripeInvoiceId).HasMaxLength(255).IsRequired();
            e.Property(x => x.StripeCustomerId).HasMaxLength(100).IsRequired();
            e.Property(x => x.StripeSubscriptionId).HasMaxLength(100);
            e.Property(x => x.Status).HasMaxLength(20).IsRequired();
            e.Property(x => x.Currency).HasMaxLength(3).IsRequired();
            e.Property(x => x.HostedInvoiceUrl).HasMaxLength(500);
            e.HasIndex(x => x.StripeInvoiceId).IsUnique();
            e.HasIndex(x => new { x.OrgId, x.CreatedAt });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<PaymentMethod>(e =>
        {
            e.ToTable("payment_methods");
            e.HasKey(x => x.Id);
            e.Property(x => x.StripePaymentMethodId).HasMaxLength(100).IsRequired();
            e.Property(x => x.StripeCustomerId).HasMaxLength(100).IsRequired();
            e.Property(x => x.Brand).HasMaxLength(20).IsRequired();
            e.Property(x => x.Last4).HasMaxLength(4).IsRequired();
            e.HasIndex(x => x.StripePaymentMethodId).IsUnique();
            e.HasIndex(x => new { x.OrgId, x.AttachedAt });
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<StripeWebhookEvent>(e =>
        {
            e.ToTable("stripe_webhook_events");
            e.HasKey(x => x.EventId);
            e.Property(x => x.EventId).HasColumnName("event_id").HasMaxLength(255).IsRequired();
            e.Property(x => x.EventType).HasColumnName("event_type").HasMaxLength(100).IsRequired();
            e.Property(x => x.ReceivedAt).HasColumnName("received_at").IsRequired();
            e.Property(x => x.ProcessedAt).HasColumnName("processed_at");
            e.Property(x => x.Status).HasColumnName("status").HasMaxLength(20).IsRequired();
            e.Property(x => x.PayloadJson).HasColumnName("payload").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.PayloadJson).HasColumnType("jsonb");
            e.Property(x => x.AttemptCount).HasColumnName("attempt_count").IsRequired();
            e.Property(x => x.LastError).HasColumnName("last_error");
            e.Property(x => x.LastErrorAt).HasColumnName("last_error_at");
            e.HasIndex(x => new { x.Status, x.ReceivedAt })
                .HasDatabaseName("idx_webhook_events_status_received");
            e.HasIndex(x => new { x.EventType, x.ReceivedAt })
                .HasDatabaseName("idx_webhook_events_type_received");
        });

        b.Entity<GithubWebhookEvent>(e =>
        {
            e.ToTable("github_webhook_events");
            e.HasKey(x => x.DeliveryId);
            e.Property(x => x.DeliveryId).HasColumnName("delivery_id").ValueGeneratedNever();
            if (Database.IsNpgsql())
                e.Property(x => x.DeliveryId).HasColumnType("uuid");
            e.Property(x => x.EventType).HasColumnName("event_type").HasMaxLength(50).IsRequired();
            e.Property(x => x.Action).HasColumnName("action").HasMaxLength(50);
            e.Property(x => x.ReceivedAt).HasColumnName("received_at").IsRequired();
            e.Property(x => x.ProcessedAt).HasColumnName("processed_at");
            e.Property(x => x.Status).HasColumnName("status").HasMaxLength(20).IsRequired();
            e.Property(x => x.PayloadJson).HasColumnName("payload").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.PayloadJson).HasColumnType("jsonb");
            e.Property(x => x.AttemptCount).HasColumnName("attempt_count").IsRequired();
            e.Property(x => x.LastError).HasColumnName("last_error");
            e.Property(x => x.LastErrorAt).HasColumnName("last_error_at");
            e.HasIndex(x => new { x.Status, x.ReceivedAt })
                .HasDatabaseName("idx_github_webhook_events_status_received");
            e.HasIndex(x => new { x.EventType, x.ReceivedAt })
                .HasDatabaseName("idx_github_webhook_events_type_received");
        });

        b.Entity<GithubInstallation>(e =>
        {
            e.ToTable("github_installations");
            e.HasKey(x => x.InstallationId);
            e.Property(x => x.InstallationId).HasColumnName("installation_id")
                .ValueGeneratedNever();
            e.Property(x => x.AppId).HasColumnName("app_id").IsRequired();
            e.Property(x => x.OrgId).HasColumnName("org_id");
            e.Property(x => x.AccountLogin).HasColumnName("account_login").HasMaxLength(255).IsRequired();
            e.Property(x => x.AccountType).HasColumnName("account_type").HasMaxLength(20).IsRequired();
            e.Property(x => x.RepoSelection).HasColumnName("repo_selection").HasMaxLength(20).IsRequired();
            e.Property(x => x.RepoSetJson).HasColumnName("repo_set").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.RepoSetJson).HasColumnType("jsonb");
            e.Property(x => x.InstalledAt).HasColumnName("installed_at").IsRequired();
            e.Property(x => x.ClaimedAt).HasColumnName("claimed_at");
            e.Property(x => x.SuspendedAt).HasColumnName("suspended_at");
            e.Property(x => x.DeletedAt).HasColumnName("deleted_at");
            e.Property(x => x.LastReconciledAt).HasColumnName("last_reconciled_at").IsRequired();

            // Cross-tenant guardrail: at most one active installation per org (spec :8427, :10024).
            e.HasIndex(x => x.OrgId)
                .HasDatabaseName("idx_github_installations_org")
                .HasFilter("\"deleted_at\" IS NULL AND \"org_id\" IS NOT NULL")
                .IsUnique();
            e.HasIndex(x => x.AppId).HasDatabaseName("idx_github_installations_app");
            e.HasIndex(x => x.InstallationId)
                .HasDatabaseName("idx_github_installations_alive")
                .HasFilter("\"deleted_at\" IS NULL");

            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId)
                .OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<PasswordResetToken>(e =>
        {
            e.ToTable("password_reset_tokens");
            e.HasKey(x => x.Id);
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            e.Property(x => x.TokenHash).HasColumnName("token_hash").IsRequired();
            e.Property(x => x.IssuedAt).HasColumnName("issued_at").IsRequired();
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at").IsRequired();
            e.Property(x => x.ConsumedAt).HasColumnName("consumed_at");
            e.Property(x => x.RevokedAt).HasColumnName("revoked_at");
            e.Property(x => x.RequesterIp).HasColumnName("requester_ip");
            e.Property(x => x.RequesterUa).HasColumnName("requester_ua");
            if (Database.IsNpgsql())
                e.Property(x => x.RequesterIp).HasColumnType("inet");
            e.HasIndex(x => x.TokenHash)
                .IsUnique()
                .HasDatabaseName("idx_password_reset_tokens_hash");
            e.HasIndex(x => new { x.UserId, x.IssuedAt })
                .HasDatabaseName("idx_password_reset_tokens_user")
                .IsDescending(false, true);
            e.HasIndex(x => x.ExpiresAt)
                .HasDatabaseName("idx_password_reset_tokens_active")
                .HasFilter("\"consumed_at\" IS NULL AND \"revoked_at\" IS NULL");
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<EmailVerificationToken>(e =>
        {
            e.ToTable("email_verification_tokens");
            e.HasKey(x => x.Id);
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            e.Property(x => x.TokenHash).HasColumnName("token_hash").IsRequired();
            e.Property(x => x.IssuedAt).HasColumnName("issued_at").IsRequired();
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at").IsRequired();
            e.Property(x => x.ConsumedAt).HasColumnName("consumed_at");
            e.Property(x => x.RevokedAt).HasColumnName("revoked_at");
            e.HasIndex(x => x.TokenHash)
                .IsUnique()
                .HasDatabaseName("idx_email_verification_tokens_hash");
            e.HasIndex(x => new { x.UserId, x.IssuedAt })
                .HasDatabaseName("idx_email_verification_tokens_user")
                .IsDescending(false, true);
            e.HasIndex(x => x.ExpiresAt)
                .HasDatabaseName("idx_email_verification_tokens_active")
                .HasFilter("\"consumed_at\" IS NULL AND \"revoked_at\" IS NULL");
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        // ── Trials (M16-005) ─────────────────────────────────────────────────────

        b.Entity<Trial>(e =>
        {
            e.ToTable("trials");
            e.HasKey(x => x.Id);
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            e.Property(x => x.Feature).HasColumnName("feature").HasMaxLength(64).IsRequired();
            e.Property(x => x.Kind)
                .HasColumnName("kind")
                .HasMaxLength(32)
                .IsRequired()
                .HasConversion(new TrialKindConverter());
            e.Property(x => x.GrantedAt).HasColumnName("granted_at").IsRequired();
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at").IsRequired();
            e.Property(x => x.ConsumedAt).HasColumnName("consumed_at");
            e.Property(x => x.Notified3DayAt).HasColumnName("notified_3day_at");
            e.Property(x => x.Notified1DayAt).HasColumnName("notified_1day_at");
            e.Property(x => x.CreatedAt).HasColumnName("created_at").IsRequired();
            e.Property(x => x.UpdatedAt).HasColumnName("updated_at").IsRequired();

            // CHECK constraint enforces the spec's three-value enum at the DDL level.
            e.ToTable(t => t.HasCheckConstraint(
                "ck_trials_kind",
                "\"kind\" IN ('full_initial', 'ondemand', 'preempted_by_subscription')"));

            // UNIQUE (user_id, feature) — one trial per feature per user, ever.
            e.HasIndex(x => new { x.UserId, x.Feature })
                .IsUnique()
                .HasDatabaseName("idx_trials_user_feature");

            // Non-unique covering index on user_id for "all my trials" lookups.
            e.HasIndex(x => x.UserId)
                .HasDatabaseName("idx_trials_user");

            // Partial indexes for the daily TrialExpiryNotifier cron (M16-008).
            // EF Core tracks only one HasIndex per property, so idx_trials_notify_3day
            // is created via raw SQL in the migration (see 20260510132915_AddTrials.cs).
            e.HasIndex(x => x.ExpiresAt)
                .HasDatabaseName("idx_trials_notify_1day")
                .HasFilter("\"notified_1day_at\" IS NULL AND \"consumed_at\" IS NULL");

            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        // ── GitLab (M16-013) ──────────────────────────────────────────────────────

        b.Entity<GitLabInstallation>(e =>
        {
            e.ToTable("gitlab_installations");
            e.HasKey(x => x.Id);
            e.Property(x => x.OrgId).HasColumnName("org_id").IsRequired();
            e.Property(x => x.ProjectId).HasColumnName("project_id").IsRequired();
            e.Property(x => x.ProjectPath).HasColumnName("project_path").HasMaxLength(500).IsRequired();
            e.Property(x => x.GitLabBaseUrl).HasColumnName("gitlab_base_url").HasMaxLength(500).IsRequired();
            e.Property(x => x.GitLabCaBundle).HasColumnName("gitlab_ca_bundle");
            e.Property(x => x.AccessTokenKid).HasColumnName("access_token_kid").HasMaxLength(500).IsRequired();
            e.Property(x => x.AccessTokenRevokedAt).HasColumnName("access_token_revoked_at");
            e.Property(x => x.WebhookSecretRotatedAt).HasColumnName("webhook_secret_rotated_at");
            e.Property(x => x.CreatedAt).HasColumnName("created_at").IsRequired();
            e.Property(x => x.UpdatedAt).HasColumnName("updated_at").IsRequired();
            e.Property(x => x.DeletedAt).HasColumnName("deleted_at");

            // bytea on Postgres, BLOB (default) on SQLite — byte[] maps automatically on SQLite
            if (Database.IsNpgsql())
            {
                e.Property(x => x.AccessTokenCiphertext).HasColumnName("access_token_ciphertext").HasColumnType("bytea").IsRequired();
                e.Property(x => x.WebhookSecretCiphertext).HasColumnName("webhook_secret_ciphertext").HasColumnType("bytea");
            }
            else
            {
                e.Property(x => x.AccessTokenCiphertext).HasColumnName("access_token_ciphertext").IsRequired();
                e.Property(x => x.WebhookSecretCiphertext).HasColumnName("webhook_secret_ciphertext");
            }

            // Partial unique index: (org_id, project_id, gitlab_base_url) WHERE deleted_at IS NULL
            // A given project in a given GitLab instance can only have one active integration per org.
            e.HasIndex(x => new { x.OrgId, x.ProjectId, x.GitLabBaseUrl })
                .HasDatabaseName("idx_gitlab_installations_org_project_base")
                .HasFilter("\"deleted_at\" IS NULL")
                .IsUnique();

            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
        });

        b.Entity<GitLabWebhookEvent>(e =>
        {
            e.ToTable("gitlab_webhook_events");
            e.HasKey(x => x.Id);
            e.Property(x => x.EventUuid).HasColumnName("event_uuid").HasMaxLength(255).IsRequired();
            e.Property(x => x.EventType).HasColumnName("event_type").HasMaxLength(100).IsRequired();
            e.Property(x => x.InstallationId).HasColumnName("installation_id").IsRequired();
            e.Property(x => x.ReceivedAt).HasColumnName("received_at").IsRequired();
            e.Property(x => x.ProcessedAt).HasColumnName("processed_at");
            e.Property(x => x.FailureCount).HasColumnName("failure_count").HasDefaultValue(0);
            e.Property(x => x.QuarantinedAt).HasColumnName("quarantined_at");

            // jsonb on Postgres, TEXT on SQLite (EF default for string)
            if (Database.IsNpgsql())
                e.Property(x => x.PayloadJson).HasColumnName("payload").HasColumnType("jsonb").IsRequired();
            else
                e.Property(x => x.PayloadJson).HasColumnName("payload").IsRequired();

            // Idempotency: X-Gitlab-Event-UUID must be unique
            e.HasIndex(x => x.EventUuid)
                .HasDatabaseName("idx_gitlab_webhook_events_uuid")
                .IsUnique();

            // FK with ON DELETE CASCADE (Decision E — orphan events have no business value
            // when the installation is hard-deleted; matches the project's cascade pattern)
            e.HasOne<GitLabInstallation>().WithMany()
                .HasForeignKey(x => x.InstallationId)
                .OnDelete(DeleteBehavior.Cascade);
        });

        // ── TeamVaults (M16-017, M18-009) ─────────────────────────────────────────

        b.Entity<TeamVault>(e =>
        {
            e.ToTable("team_vaults");
            e.HasKey(x => x.OrgId);
            e.Property(x => x.OrgId).HasColumnName("org_id").IsRequired();
            e.Property(x => x.TemplateYaml).HasColumnName("template_yaml").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.TemplateJson).HasColumnName("template_jsonb").HasColumnType("jsonb").IsRequired();
            else
                e.Property(x => x.TemplateJson).HasColumnName("template_jsonb").IsRequired();
            e.Property(x => x.Version).HasColumnName("version").IsRequired().HasDefaultValue(1L);
            e.Property(x => x.CreatedAt).HasColumnName("created_at").IsRequired();
            e.Property(x => x.CreatedBy).HasColumnName("created_by").IsRequired(false);
            e.Property(x => x.UpdatedAt).HasColumnName("updated_at").IsRequired();
            e.Property(x => x.UpdatedBy).HasColumnName("updated_by").IsRequired(false);
            // M18-009: envelope-encrypted columns (nullable during expand-contract window)
            if (Database.IsNpgsql())
                e.Property(x => x.TemplateJsonCiphertext).HasColumnName("template_jsonb_ciphertext").HasColumnType("bytea").IsRequired(false);
            else
                e.Property(x => x.TemplateJsonCiphertext).HasColumnName("template_jsonb_ciphertext").IsRequired(false);
            e.Property(x => x.TemplateJsonKid).HasColumnName("template_jsonb_kid").HasMaxLength(500).IsRequired(false);
            e.HasIndex(x => x.UpdatedAt).HasDatabaseName("idx_team_vaults_updated");
            e.HasOne<Organization>().WithMany().HasForeignKey(x => x.OrgId).OnDelete(DeleteBehavior.Cascade);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.CreatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UpdatedBy).IsRequired(false).OnDelete(DeleteBehavior.SetNull);
        });

        // ── UserExportRequests (M18-004) ──────────────────────────────────────────

        b.Entity<UserExportRequest>(e =>
        {
            e.ToTable("user_export_requests");
            e.HasKey(x => x.Id);
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            e.Property(x => x.Status).HasConversion<string>().HasMaxLength(20).IsRequired();
            e.Property(x => x.ObjectKey).HasColumnName("object_key").HasMaxLength(500);
            e.Property(x => x.CreatedAt).HasColumnName("created_at").IsRequired();
            e.Property(x => x.ReadyAt).HasColumnName("ready_at");
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at");
            e.Property(x => x.FailureReason).HasColumnName("failure_reason").HasMaxLength(200);
            // Optimistic concurrency token: prevents two concurrent builder ticks from
            // double-processing the same row (EF will throw DbUpdateConcurrencyException on clash).
            e.Property(x => x.Version).HasColumnName("version").IsRequired()
                .IsConcurrencyToken();
            e.HasIndex(x => new { x.UserId, x.CreatedAt })
                .IsDescending(false, true)
                .HasDatabaseName("idx_user_export_requests_user_created");
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        // ── DeletionReauthTokens (M18-005) ────────────────────────────────────────

        b.Entity<DeletionReauthToken>(e =>
        {
            e.ToTable("deletion_reauth_tokens");
            e.HasKey(x => x.Id);
            e.Property(x => x.UserId).HasColumnName("user_id").IsRequired();
            e.Property(x => x.TokenHash).HasColumnName("token_hash").IsRequired();
            e.Property(x => x.IssuedAt).HasColumnName("issued_at").IsRequired();
            e.Property(x => x.ExpiresAt).HasColumnName("expires_at").IsRequired();
            e.Property(x => x.ConsumedAt).HasColumnName("consumed_at");
            // Unique hash index — no two tokens share the same SHA-256 hash (collision-safe).
            e.HasIndex(x => x.TokenHash)
                .IsUnique()
                .HasDatabaseName("idx_deletion_reauth_tokens_hash");
            // Descending (user_id, issued_at): covers "latest token for this user" lookups.
            e.HasIndex(x => new { x.UserId, x.IssuedAt })
                .HasDatabaseName("idx_deletion_reauth_tokens_user")
                .IsDescending(false, true);
            // Partial index: live tokens (not yet consumed) ordered by expiry for cleanup.
            e.HasIndex(x => x.ExpiresAt)
                .HasDatabaseName("idx_deletion_reauth_tokens_active")
                .HasFilter("\"consumed_at\" IS NULL");
            e.HasOne<User>().WithMany().HasForeignKey(x => x.UserId).OnDelete(DeleteBehavior.Cascade);
        });

        // ── TelemetryEvents (M18-007) ─────────────────────────────────────────────

        b.Entity<TelemetryEvent>(e =>
        {
            e.ToTable("telemetry_events");
            e.HasKey(x => x.Id);
            e.Property(x => x.InstallId).HasColumnName("install_id").IsRequired();
            if (Database.IsNpgsql())
                e.Property(x => x.InstallId).HasColumnType("uuid");
            e.Property(x => x.EventType).HasColumnName("event_type").HasMaxLength(64).IsRequired();
            e.Property(x => x.ReceivedAt).HasColumnName("received_at").IsRequired();
            e.Property(x => x.IdempotencyKey).HasColumnName("idempotency_key").HasMaxLength(64).IsRequired();
            // jsonb on Postgres, TEXT on SQLite (EF default for string)
            if (Database.IsNpgsql())
                e.Property(x => x.EventPayloadJson).HasColumnName("event_payload").HasColumnType("jsonb").IsRequired();
            else
                e.Property(x => x.EventPayloadJson).HasColumnName("event_payload").IsRequired();
            // UNIQUE constraint on idempotency_key — drives at-most-once delivery semantics.
            e.HasIndex(x => x.IdempotencyKey)
                .IsUnique()
                .HasDatabaseName("idx_telemetry_events_idempotency_key");
            // Index to support the 90-day purge query (filter by received_at).
            e.HasIndex(x => x.ReceivedAt)
                .HasDatabaseName("idx_telemetry_events_received_at");
            // Index to support per-install_id rate-limit lookups and aggregation.
            e.HasIndex(x => new { x.InstallId, x.ReceivedAt })
                .HasDatabaseName("idx_telemetry_events_install_received");
        });

        // ── TelemetryDailyAggregates (M18-007) ───────────────────────────────────

        b.Entity<TelemetryDailyAggregate>(e =>
        {
            e.ToTable("telemetry_daily_aggregates");
            // Composite PK (day, event_type) — idempotency guard for re-aggregation.
            e.HasKey(x => new { x.Day, x.EventType });
            e.Property(x => x.Day).HasColumnName("day").IsRequired();
            e.Property(x => x.EventType).HasColumnName("event_type").HasMaxLength(64).IsRequired();
            e.Property(x => x.EventCount).HasColumnName("event_count").IsRequired();
            e.Property(x => x.DistinctInstallCount).HasColumnName("distinct_install_count").IsRequired();
            // jsonb on Postgres, TEXT on SQLite
            if (Database.IsNpgsql())
                e.Property(x => x.NumericSumsJson).HasColumnName("numeric_sums").HasColumnType("jsonb").IsRequired();
            else
                e.Property(x => x.NumericSumsJson).HasColumnName("numeric_sums").IsRequired();
        });
    }
}
