using System.Text.Json;
using System.Text.Json.Serialization;
using System.Threading.RateLimiting;
using ApiTool.Backend;
using ApiTool.Backend.Audit;
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Storage;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Auth.Refresh;
using ApiTool.Backend.Bootstrap;
using ApiTool.Backend.Coordinator;
using ApiTool.Backend.Internal;
using ApiTool.Backend.Internal.TierGates;
using ApiTool.Backend.Data;
using ApiTool.Backend.Health;
using ApiTool.Backend.Invitations;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Licensing.Tokens;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Notifications.Email;
using ApiTool.Backend.Notifications.Trials;
using ApiTool.Backend.Organizations;
using ApiTool.Backend.GitHub;
using ApiTool.Backend.GitHub.Installations;
using ApiTool.Backend.GitHub.Webhooks;
using ApiTool.Backend.GitLab;
using ApiTool.Backend.GitLab.Installations;
using ApiTool.Backend.GitLab.Webhooks;
using ApiTool.Backend.Logging;
using ApiTool.Backend.PrChecks;
using ApiTool.Backend.Rbac.CustomRoles;
using ApiTool.Backend.Results;
using ApiTool.Backend.Results.Dashboard;
using ApiTool.Backend.Schedules;
using ApiTool.Backend.VaultConfig;
using ApiTool.Backend.Sso;
using ApiTool.Backend.Subscriptions;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Telemetry;
using ApiTool.Backend.Webhooks;
using ApiTool.Backend.Webhooks.Handlers;
using Microsoft.AspNetCore.Authentication.JwtBearer;
using Microsoft.AspNetCore.RateLimiting;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Options;
using HttpResults = Microsoft.AspNetCore.Http.Results;

// ── Dev CLI tools (exit with code if a dev subcommand is handled) ─────────
{
    var devExit = DevCommands.Run(args);
    if (devExit.HasValue) { Environment.Exit(devExit.Value); return; }
}

var builder = WebApplication.CreateBuilder(args);

// ── Database ──────────────────────────────────────────────────────────────
// In the Testing environment the database is configured by the test host (BackendFactory).
if (!builder.Environment.IsEnvironment("Testing"))
{
    var pgHost = builder.Configuration["POSTGRES_HOST"];
    if (!string.IsNullOrEmpty(pgHost))
    {
        var pgPort = builder.Configuration["POSTGRES_PORT"] ?? "5432";
        var pgDb   = builder.Configuration["POSTGRES_DB"] ?? "apitool";
        var pgUser = builder.Configuration["POSTGRES_USER"] ?? "apitool";
        var pgPass = builder.Configuration["POSTGRES_PASSWORD"] ?? "";
        var connectionString = $"Host={pgHost};Port={pgPort};Database={pgDb};Username={pgUser};Password={pgPass}";
        builder.Services.AddDbContext<AppDbContext>(options =>
            options.UseNpgsql(connectionString, npg => npg.MigrationsAssembly("ApiTool.Backend")));
    }
    else
    {
        builder.Services.AddDbContext<AppDbContext>(options =>
            options.UseSqlite(builder.Configuration.GetConnectionString("Default")
                ?? "Data Source=data/apitool.db"));
    }
}

// ── JWT Authentication ────────────────────────────────────────────────────
builder.Services.Configure<JwtOptions>(builder.Configuration.GetSection(JwtOptions.Section));

builder.Services
    .AddAuthentication(JwtBearerDefaults.AuthenticationScheme)
    .AddJwtBearer();  // Token validation params are set by JwtBearerPostConfigurer below.

// IPostConfigureOptions runs after all ConfigureServices callbacks, so test overrides
// applied via ConfigureAppConfiguration are picked up.
builder.Services.AddSingleton<AccessTokenVerificationKeyCache>();
builder.Services.AddSingleton<IPostConfigureOptions<JwtBearerOptions>, JwtBearerPostConfigurer>();

builder.Services.AddAuthorization();

// ── HTTP Context + Services ───────────────────────────────────────────────
builder.Services.AddHttpContextAccessor();
builder.Services.AddScoped<CurrentUserAccessor>();
builder.Services.AddScoped<AuditContext>();
builder.Services.AddScoped<IAuditWriter, AuditWriter>();
builder.Services.AddScoped<AuditLogQueryService>();
builder.Services.Configure<AuditLogCleanupOptions>(
    builder.Configuration.GetSection(AuditLogCleanupOptions.Section));
builder.Services.AddScoped<OrganizationService>();
builder.Services.AddScoped<ResultsService>();
builder.Services.AddScoped<DashboardResultsService>();   // M16-019
builder.Services.AddScoped<TierGate>();   // canonical tier query implementation
builder.Services.AddScoped<ITierGate>(sp => sp.GetRequiredService<TierGate>());   // M16-001
builder.Services.AddScoped<IOrganizationTierReader>(sp => sp.GetRequiredService<TierGate>());
builder.Services.AddScoped<SchedulesService>();
builder.Services.AddScoped<ISchedulerEnqueuer>(sp => sp.GetRequiredService<SchedulesService>());
builder.Services.AddScoped<ScheduleExecutorService>();   // M16-009
builder.Services.AddScoped<VaultConfigService>();        // M16-017

// ── Notifications ─────────────────────────────────────────────────────────
builder.Services.AddHttpClient("notifications", c => c.Timeout = TimeSpan.FromSeconds(10));
builder.Services.AddScoped<NotificationsService>();
builder.Services.AddSingleton<ISlackWebhookPoster, SlackWebhookPoster>();
builder.Services.Configure<NotificationsDispatcherOptions>(o =>
{
    o.RetryDelays = [TimeSpan.FromMilliseconds(500), TimeSpan.FromSeconds(2)];
});
// Replace the noop notifier with the real dispatcher (Scoped: resolves scoped DbContext per request)
builder.Services.AddScoped<IResultIngestedNotifier, NotificationsDispatcher>();

// ── SendGrid options + sender selection ───────────────────────────────────
builder.Services.AddOptions<SendGridOptions>()
    .Bind(builder.Configuration.GetSection(SendGridOptions.Section))
    .Validate(
        o => o.Mode != "live" || !string.IsNullOrEmpty(o.ApiKey),
        "ApiTool:SendGrid:Mode=live requires ApiTool:SendGrid:ApiKey (env: APITOOL__SENDGRID__APIKEY).")
    .ValidateOnStart();

// Template loader — content-rooted at <ContentRoot>/templates/email
builder.Services.AddSingleton(sp => new EmailTemplateLoader(
    Path.Combine(builder.Environment.ContentRootPath, "templates", "email")));
builder.Services.AddSingleton<IMjmlCompiler, MjmlNetCompiler>();
builder.Services.AddSingleton<IEmailDeadLetterStore, LoggingEmailDeadLetterStore>();

var sendGridMode = builder.Configuration["ApiTool:SendGrid:Mode"] ?? "fake";
if (sendGridMode == "live")
    builder.Services.AddSingleton<ISmtpSender, SendGridSmtpSender>();
else
    builder.Services.AddSingleton<ISmtpSender, NoopSmtpSender>();

builder.Services.AddSingleton(TimeProvider.System);
builder.Services.AddSingleton<PasswordHasher>();

// ── Health probes ─────────────────────────────────────────────────────────
builder.Services.AddSingleton<IDbHealthProbe, EfDbHealthProbe>();
builder.Services.AddSingleton<IRedisHealthProbe, TcpRedisHealthProbe>();
builder.Services.AddSingleton<HealthService>();

// ── Stripe / Subscriptions ────────────────────────────────────────────────
builder.Services.Configure<StripeOptions>(builder.Configuration.GetSection(StripeOptions.Section));
// Validate at startup when live mode is configured so misconfig is visible immediately.
builder.Services.AddOptions<StripeOptions>()
    .Bind(builder.Configuration.GetSection(StripeOptions.Section))
    .Validate(
        o => o.Mode != "live" || !string.IsNullOrEmpty(o.ApiKey),
        "ApiTool:Stripe:Mode=live requires ApiTool:Stripe:ApiKey (env: APITOOL__STRIPE__APIKEY).")
    .ValidateOnStart();

var stripeMode = builder.Configuration["ApiTool:Stripe:Mode"] ?? "fake";
if (stripeMode == "live")
    builder.Services.AddSingleton<IStripeGateway, StripeGateway>();
else
    builder.Services.AddSingleton<IStripeGateway, FakeStripeGateway>();

// ── Stripe Webhooks ───────────────────────────────────────────────────────
// Bridge the spec's documented env-var spelling APITOOL__STRIPE__WEBHOOK_SECRETS
// (single underscore between WEBHOOK and SECRETS) into the canonical config key.
var webhookSecretsAlias = builder.Configuration["APITOOL__STRIPE__WEBHOOK_SECRETS"];
if (!string.IsNullOrEmpty(webhookSecretsAlias))
{
    builder.Configuration.AddInMemoryCollection(new Dictionary<string, string?>
    {
        ["ApiTool:Stripe:Webhook:Secrets"] = webhookSecretsAlias,
    });
}

builder.Services.AddOptions<StripeWebhookOptions>()
    .Bind(builder.Configuration.GetSection(StripeWebhookOptions.Section))
    .Validate(
        o => o.ToleranceSeconds >= 30,
        "ApiTool:Stripe:Webhook:ToleranceSeconds must be >= 30 (env: APITOOL__STRIPE__WEBHOOK__TOLERANCESECONDS). " +
        "Setting to 0 disables Stripe.Net timestamp verification entirely (spec :6854).")
    .Validate(
        o => stripeMode != "live" || o.SecretList().Count > 0,
        "ApiTool:Stripe:Mode=live requires at least one webhook secret in APITOOL__STRIPE__WEBHOOK_SECRETS.")
    .ValidateOnStart();

builder.Services.Configure<StripeWebhookCleanupOptions>(
    builder.Configuration.GetSection(StripeWebhookCleanupOptions.Section));

builder.Services.AddScoped<StripeWebhookStore>();
// M14-012: real dispatcher replacing the M14-011 no-op stub.
// NoopStripeWebhookDispatcher stays available for tests that override DI.
builder.Services.AddScoped<StripeSubscriptionHandler>();
builder.Services.AddScoped<StripeCustomerUpdatedHandler>();
builder.Services.AddScoped<StripeInvoiceHandler>();          // M14-013
builder.Services.AddScoped<StripePaymentMethodHandler>();    // M14-013
builder.Services.AddScoped<IStripeWebhookDispatcher, StripeWebhookDispatcher>();

if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<StripeWebhookCleanupHost>();

// M16-003: prune password_reset_tokens + email_verification_tokens on 1h tick.
builder.Services.Configure<AuthTokenCleanupOptions>(
    builder.Configuration.GetSection(AuthTokenCleanupOptions.Section));
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<AuthTokenCleanupService>();

// ── VaultConfig options (M16-017) ─────────────────────────────────────────
// Bridge env-var spellings: VAULTCONFIG__VALIDATOR_MODE → canonical key.
{
    var modeAlias = Environment.GetEnvironmentVariable("VAULTCONFIG__VALIDATOR_MODE");
    if (!string.IsNullOrEmpty(modeAlias))
        builder.Configuration["ApiTool:VaultConfig:ValidatorMode"] = modeAlias;
}
builder.Services.Configure<VaultConfigOptions>(
    builder.Configuration.GetSection(VaultConfigOptions.Section));

// ── App-wide options ──────────────────────────────────────────────────────
builder.Services.AddOptions<AppOptions>()
    .Bind(builder.Configuration.GetSection(AppOptions.Section))
    .Validate(
        o => !string.IsNullOrWhiteSpace(o.WebAppUrl),
        "ApiTool:App:WebAppUrl is required (env: APITOOL__APP__WEBAPPURL).")
    .ValidateOnStart();

// ── License signing keys ──────────────────────────────────────────────────
builder.Services.Configure<KeyProviderOptions>(
    builder.Configuration.GetSection(KeyProviderOptions.Section));
builder.Services.AddScoped<ISigningKeyStore, EfSigningKeyStore>();
var keyMode = builder.Configuration["ApiTool:KeyProvider:Mode"] ?? "file";
if (keyMode == "kms")
{
    builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));
    builder.Services.AddSingleton<IKmsClient, GoogleKmsClient>();
    builder.Services.AddScoped<IKeyProvider, GoogleKmsKeyProvider>();
}
else
{
    builder.Services.AddScoped<IKeyProvider, FileKeyProvider>();
}
builder.Services.AddScoped<ISigningKeyRotator, SigningKeyRotator>();

// ── GitHub Installations service + webhook handlers ───────────────────────
builder.Services.AddScoped<GithubInstallationsService>();
builder.Services.AddScoped<GithubInstallationCreatedHandler>();
builder.Services.AddScoped<GithubInstallationRepositoriesHandler>();
builder.Services.Configure<GithubInstallationReconcilerOptions>(
    builder.Configuration.GetSection(GithubInstallationReconcilerOptions.Section));
// Real API client — uses IInstallationTokenCache (registered below in M14-018 block).
// In Testing the reconciler host is not started, so tests inject their own
// IGitHubInstallationsApi directly via BackendFactory.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddSingleton<IGitHubInstallationsApi, GitHubInstallationsApi>();
    builder.Services.AddHostedService<GithubInstallationReconcilerHost>();
}

// ── GitHub App key provider ────────────────────────────────────────────────
// Refs docs/SPECIFICATION.md:8350-8388. RS256 lives ONLY here (Decision #13, :8359).
builder.Services.AddOptions<GitHubAppOptions>()
    .Bind(builder.Configuration.GetSection(GitHubAppOptions.Section))
    .Validate(o =>
    {
        // Require key path/id only when the GitHub App is actually configured (AppId != 0).
        if (o.AppId == 0) return true;
        return o.KeyProvider.Mode switch
        {
            "file" => !string.IsNullOrEmpty(o.KeyProvider.File.Path),
            "kms"  => !string.IsNullOrEmpty(o.KeyProvider.Kms.KmsKeyId),
            _      => false,
        };
    }, "ApiTool:GitHubApp:KeyProvider configuration is invalid for the selected mode.")
    .Validate(o =>
    {
        // StateSigningKey is required when the GitHub App is configured.
        // An empty key would allow anyone with knowledge of the default to forge state tokens.
        if (o.AppId == 0) return true;
        return !string.IsNullOrEmpty(o.StateSigningKey);
    }, "ApiTool:GitHubApp:StateSigningKey is required when AppId != 0 (env: APITOOL__GITHUBAPP__STATESIGNINGKEY).")
    .ValidateOnStart();

var ghAppMode = builder.Configuration["ApiTool:GitHubApp:KeyProvider:Mode"] ?? "file";
if (ghAppMode == "kms")
{
    // Ensure IKmsClient is registered even when the License-key path uses a different mode
    // (keyMode and ghAppMode are independent env vars). Avoid double-registration if both
    // use KMS by checking whether the service is already registered.
    if (keyMode != "kms")
    {
        builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));
        builder.Services.AddSingleton<IKmsClient, GoogleKmsClient>();
    }
    builder.Services.AddSingleton<IGitHubAppKeyProvider, GoogleKmsGitHubAppKeyProvider>();
}
else
{
    builder.Services.AddSingleton<IGitHubAppKeyProvider, FileGitHubAppKeyProvider>();
}

// ── GitLab key provider (M16-013) ─────────────────────────────────────────
// Refs docs/SPECIFICATION.md:9196-9205. AES-256-GCM envelope with per-row DEK.
// Bridge env-var spellings from the task observable (GITLAB__KEY_PROVIDER, GITLAB__KEK_PATH).
{
    var modeAlias = Environment.GetEnvironmentVariable("GITLAB__KEY_PROVIDER");
    if (!string.IsNullOrEmpty(modeAlias))
        builder.Configuration["ApiTool:GitLab:KeyProvider:Mode"] = modeAlias;
    var kekAlias = Environment.GetEnvironmentVariable("GITLAB__KEK_PATH");
    if (!string.IsNullOrEmpty(kekAlias))
        builder.Configuration["ApiTool:GitLab:KeyProvider:File:KekPath"] = kekAlias;
    var kmsAlias = Environment.GetEnvironmentVariable("GITLAB__KMS_KEY_ID");
    if (!string.IsNullOrEmpty(kmsAlias))
        builder.Configuration["ApiTool:GitLab:KeyProvider:Kms:KmsKeyId"] = kmsAlias;
    // M16-014: HTTPS enforcement escape-hatch for dev/test (Open Decision 2)
    var allowHttpAlias = Environment.GetEnvironmentVariable("GITLAB__ALLOW_HTTP");
    if (!string.IsNullOrEmpty(allowHttpAlias))
        builder.Configuration["ApiTool:GitLab:Poster:AllowHttp"] = allowHttpAlias;
}

// GitLab options are optional — only validate when the provider is explicitly configured.
// FileGitLabKeyProvider validates the KEK path at construction time; this startup check
// only applies in non-Testing environments where the provider is actually instantiated.
builder.Services.AddOptions<GitLabOptions>()
    .Bind(builder.Configuration.GetSection(GitLabOptions.Section));

var glProviderMode = builder.Configuration["ApiTool:GitLab:KeyProvider:Mode"] ?? "file";
if (glProviderMode == "kms")
{
    // Avoid double-registration of IKmsClient/HttpClient if License or GitHub App already uses KMS.
    if (keyMode != "kms" && ghAppMode != "kms")
    {
        builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));
        builder.Services.AddSingleton<IKmsClient, GoogleKmsClient>();
    }
    builder.Services.AddSingleton<IGitLabKeyProvider, GoogleKmsGitLabKeyProvider>();
}
else if (!builder.Environment.IsEnvironment("Testing"))
{
    // In Testing, BackendFactory registers FakeGitLabKeyProvider to avoid real file I/O.
    builder.Services.AddSingleton<IGitLabKeyProvider, FileGitLabKeyProvider>();
}

// ── TeamVault key provider (M18-009, v4-12) ──────────────────────────────
// AES-256-GCM envelope with per-row DEK for team_vaults.template_jsonb_ciphertext.
// Bridge env-var spellings: TEAMVAULT__KEY_PROVIDER, TEAMVAULT__KEK_PATH, TEAMVAULT__KMS_KEY_ID.
{
    var modeAlias = Environment.GetEnvironmentVariable("TEAMVAULT__KEY_PROVIDER");
    if (!string.IsNullOrEmpty(modeAlias))
        builder.Configuration["ApiTool:TeamVault:Encryption:KeyProvider:Mode"] = modeAlias;
    var kekAlias = Environment.GetEnvironmentVariable("TEAMVAULT__KEK_PATH");
    if (!string.IsNullOrEmpty(kekAlias))
        builder.Configuration["ApiTool:TeamVault:Encryption:KeyProvider:File:KekPath"] = kekAlias;
    var kmsAlias = Environment.GetEnvironmentVariable("TEAMVAULT__KMS_KEY_ID");
    if (!string.IsNullOrEmpty(kmsAlias))
        builder.Configuration["ApiTool:TeamVault:Encryption:KeyProvider:Kms:KmsKeyId"] = kmsAlias;
}
builder.Services.AddOptions<ApiTool.Backend.VaultConfig.Keys.TeamVaultEncryptionOptions>()
    .Bind(builder.Configuration.GetSection(ApiTool.Backend.VaultConfig.Keys.TeamVaultEncryptionOptions.Section));
{
    var tvMode = builder.Configuration["ApiTool:TeamVault:Encryption:KeyProvider:Mode"] ?? "file";
    if (tvMode == "kms")
    {
        if (keyMode != "kms" && ghAppMode != "kms" && glProviderMode != "kms")
        {
            builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));
            builder.Services.AddSingleton<IKmsClient, GoogleKmsClient>();
        }
        builder.Services.AddSingleton<ApiTool.Backend.VaultConfig.Keys.ITeamVaultKeyProvider,
            ApiTool.Backend.VaultConfig.Keys.GoogleKmsTeamVaultKeyProvider>();
    }
    else if (!builder.Environment.IsEnvironment("Testing"))
    {
        // In Testing, SqliteBackendFactory registers FakeTeamVaultKeyProvider.
        builder.Services.AddSingleton<ApiTool.Backend.VaultConfig.Keys.ITeamVaultKeyProvider,
            ApiTool.Backend.VaultConfig.Keys.FileTeamVaultKeyProvider>();
    }
}

// ── ScheduleEnv key provider (M18-009, v4-12) ────────────────────────────
// AES-256-GCM envelope with per-row DEK for schedules.env_vars_ciphertext.
// Bridge env-var spellings: SCHEDULES__KEY_PROVIDER, SCHEDULES__KEK_PATH, SCHEDULES__KMS_KEY_ID.
{
    var modeAlias = Environment.GetEnvironmentVariable("SCHEDULES__KEY_PROVIDER");
    if (!string.IsNullOrEmpty(modeAlias))
        builder.Configuration["ApiTool:Schedules:Encryption:KeyProvider:Mode"] = modeAlias;
    var kekAlias = Environment.GetEnvironmentVariable("SCHEDULES__KEK_PATH");
    if (!string.IsNullOrEmpty(kekAlias))
        builder.Configuration["ApiTool:Schedules:Encryption:KeyProvider:File:KekPath"] = kekAlias;
    var kmsAlias = Environment.GetEnvironmentVariable("SCHEDULES__KMS_KEY_ID");
    if (!string.IsNullOrEmpty(kmsAlias))
        builder.Configuration["ApiTool:Schedules:Encryption:KeyProvider:Kms:KmsKeyId"] = kmsAlias;
}
builder.Services.AddOptions<ApiTool.Backend.Schedules.Keys.ScheduleEnvEncryptionOptions>()
    .Bind(builder.Configuration.GetSection(ApiTool.Backend.Schedules.Keys.ScheduleEnvEncryptionOptions.Section));
{
    var seMode = builder.Configuration["ApiTool:Schedules:Encryption:KeyProvider:Mode"] ?? "file";
    if (seMode == "kms")
    {
        if (keyMode != "kms" && ghAppMode != "kms" && glProviderMode != "kms")
        {
            builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));
            builder.Services.AddSingleton<IKmsClient, GoogleKmsClient>();
        }
        builder.Services.AddSingleton<ApiTool.Backend.Schedules.Keys.IScheduleEnvKeyProvider,
            ApiTool.Backend.Schedules.Keys.GoogleKmsScheduleEnvKeyProvider>();
    }
    else if (!builder.Environment.IsEnvironment("Testing"))
    {
        // In Testing, SqliteBackendFactory registers FakeScheduleEnvKeyProvider.
        builder.Services.AddSingleton<ApiTool.Backend.Schedules.Keys.IScheduleEnvKeyProvider,
            ApiTool.Backend.Schedules.Keys.FileScheduleEnvKeyProvider>();
    }
}

// ── GitHub named HttpClients + CheckRunPoster (M14-018) ───────────────────
// Named client "github-app": used for App-JWT → installation token exchange
// Named client "github-checks": used for POST /repos/{o}/{r}/check-runs
var githubApiBase = builder.Configuration["GitHub__ApiBase"]
    ?? builder.Configuration["ApiTool:GitHub:ApiBase"]
    ?? "https://api.github.com";
builder.Services.AddHttpClient("github-app", c =>
{
    c.BaseAddress = new Uri(githubApiBase);
    c.Timeout = TimeSpan.FromSeconds(15);
    c.DefaultRequestHeaders.Add("User-Agent", "ApiTool-Backend/1.0");
});
builder.Services.AddHttpClient("github-checks", c =>
{
    c.BaseAddress = new Uri(githubApiBase);
    c.Timeout = TimeSpan.FromSeconds(10);
    c.DefaultRequestHeaders.Add("User-Agent", "ApiTool-Backend/1.0");
});
builder.Services.AddSingleton<IInstallationTokenCache, InstallationTokenCache>();
builder.Services.AddSingleton<IRateLimitTracker, RateLimitTracker>();
builder.Services.AddScoped<ICheckRunPoster, CheckRunPoster>();

// ── GitLab outbound poster (M16-014) ─────────────────────────────────────────
// Named client "gitlab-statuses": used for POST /api/v4/projects/{id}/statuses/{sha}.
// No BaseAddress — per-installation gitlab_base_url is resolved at request time.
builder.Services.AddHttpClient("gitlab-statuses", c =>
{
    c.Timeout = TimeSpan.FromSeconds(10);
    c.DefaultRequestHeaders.Add("User-Agent", "ApiTool-Backend/1.0");
});
builder.Services.AddSingleton<IGitLabRateLimitTracker, GitLabRateLimitTracker>();
builder.Services.AddScoped<IGitLabCheckPoster, GitLabCheckPoster>();

// Named client "gitlab-projects": one-shot project_id lookup during integration creation (M16-016).
// No BaseAddress — per-installation gitlab_base_url is resolved at request time.
builder.Services.AddHttpClient("gitlab-projects", c =>
{
    c.Timeout = TimeSpan.FromSeconds(10);
    c.DefaultRequestHeaders.Add("User-Agent", "ApiTool-Backend/1.0");
});
builder.Services.AddScoped<IGitLabProjectLookup, GitLabProjectLookup>();

// ── GitHub Webhook ingest pipeline (M14-019) ──────────────────────────────
// Accept the env-var spelling shown in the task observable ('GITHUB__WEBHOOK_SECRETS'
// without the APITOOL prefix) by feeding it into the canonical config key.
{
    var legacy = Environment.GetEnvironmentVariable("GITHUB__WEBHOOK_SECRETS");
    if (!string.IsNullOrWhiteSpace(legacy))
        builder.Configuration["ApiTool:GitHub:Webhook:Secrets"] = legacy;
}

builder.Services.AddOptions<GithubWebhookOptions>()
    .Bind(builder.Configuration.GetSection(GithubWebhookOptions.Section));
builder.Services.Configure<GithubWebhookCleanupOptions>(
    builder.Configuration.GetSection(GithubWebhookCleanupOptions.Section));

builder.Services.AddScoped<GithubWebhookStore>();
builder.Services.AddScoped<GithubInstallationLifecycleHandler>();
builder.Services.AddSingleton<IRerunJobQueue, LoggingRerunJobQueue>();
builder.Services.AddScoped<GithubCheckRunHandler>();
builder.Services.AddScoped<IGithubWebhookDispatcher, GithubWebhookDispatcher>();

if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<GithubWebhookCleanupHost>();

// ── GitLab Webhook ingest pipeline (M16-015) ──────────────────────────────
// Single-secret-per-installation only (Open Decision 3).
// Multi-secret rotation via GITLAB__WEBHOOK_SECRETS_<installation_id> is a documented follow-up.
builder.Services.Configure<GitLabWebhookCleanupOptions>(
    builder.Configuration.GetSection(GitLabWebhookCleanupOptions.Section));

builder.Services.AddScoped<GitLabWebhookStore>();
builder.Services.AddSingleton<GitLabWebhookSourceTracker>();
builder.Services.AddScoped<GitLabPipelineHookHandler>();
builder.Services.AddScoped<IGitLabWebhookDispatcher, GitLabWebhookDispatcher>();

if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<GitLabWebhookCleanupHost>();

// ── Object store (M18-004) ────────────────────────────────────────────────
// In Testing the InMemoryObjectStore is always used regardless of config.
// In all other environments, the Provider config key selects the implementation:
//   in_memory — test-only ConcurrentDictionary store (default in Testing)
//   s3        — Amazon S3 / MinIO (self-hosted default)
//   gcs       — Google Cloud Storage (SaaS profile)
builder.Services.Configure<ObjectStoreOptions>(
    builder.Configuration.GetSection(ObjectStoreOptions.Section));
// The deployment-facing variables intentionally omit the ApiTool__ prefix
// (OBJECTSTORE__PROVIDER, OBJECTSTORE__S3__ENDPOINT, ...). Overlay that section
// after appsettings so explicit environment values win while retaining defaults.
builder.Services.Configure<ObjectStoreOptions>(
    builder.Configuration.GetSection("OBJECTSTORE"));
{
    var objectStoreProvider = builder.Environment.IsEnvironment("Testing")
        ? "in_memory"
        : (builder.Configuration["OBJECTSTORE:PROVIDER"]
            ?? builder.Configuration["ApiTool:ObjectStore:Provider"]
            ?? "in_memory");

    if (objectStoreProvider == "in_memory")
        builder.Services.AddSingleton<IObjectStore, InMemoryObjectStore>();
    else if (objectStoreProvider == "s3")
        builder.Services.AddSingleton<IObjectStore, S3ObjectStore>();
    else if (objectStoreProvider == "gcs")
        builder.Services.AddSingleton<IObjectStore, GcsObjectStore>();
    else
        throw new InvalidOperationException(
            $"ObjectStore provider '{objectStoreProvider}' is not supported. " +
            "Valid values: 'in_memory', 's3', 'gcs' (env: OBJECTSTORE__PROVIDER).");
}

// ── Audit log cleanup host (M18-002) ─────────────────────────────────────
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<AuditLogCleanupHost>();

// ── User export builder host (M18-004) ────────────────────────────────────
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<ApiTool.Backend.Compliance.Gdpr.UserExportBuilderHost>();

// ── GDPR deletion re-auth service + anonymiser + finalizer host (M18-005) ──
builder.Services.AddScoped<IDeletionReauthService, DeletionReauthService>();
builder.Services.AddScoped<IUserAnonymiser, UserAnonymiser>();
builder.Services.AddScoped<LastAdminProtectionService>();  // M18-006
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<UserDeletionFinalizerHost>();

// ── Logging redaction (precondition for M14-018) ──────────────────────────
// Refs docs/SPECIFICATION.md:8466 + :8631.
// ClearProviders() removes the default unredacted ConsoleLoggerProvider that
// WebApplication.CreateBuilder() adds automatically. Then the wrapped provider
// is added as the ONLY console sink so every log line is scrubbed before
// emission. Without ClearProviders() the original unredacted provider stays
// active alongside the wrapper and tokens appear in plain text on one of the
// two resulting output lines.
// Note: Debug and EventSource providers are re-added explicitly so dev/trace
// tooling continues to work. Additional sinks (ApplicationInsights, file) will
// be wrapped with BearerTokenRedactor when they are introduced in later milestones.
builder.Logging.ClearProviders();
builder.Logging.AddDebug();
builder.Logging.AddEventSourceLogger();
// Register the BearerTokenRedactor-wrapped console provider via AddSingleton so the
// DI container resolves IOptionsMonitor<ConsoleLoggerOptions> at first-use time (not
// at registration time), avoiding the BuildServiceProvider() anti-pattern.
builder.Services.AddSingleton<ILoggerProvider>(sp =>
{
    var consoleOptions = sp.GetRequiredService<IOptionsMonitor<Microsoft.Extensions.Logging.Console.ConsoleLoggerOptions>>();
    var inner = new Microsoft.Extensions.Logging.Console.ConsoleLoggerProvider(consoleOptions);
    return new BearerTokenRedactor(inner);
});

// ── License token issuance ───────────────────────────────────────────────
builder.Services.Configure<TokenIssuerOptions>(
    builder.Configuration.GetSection(TokenIssuerOptions.Section));
builder.Services.AddScoped<LicenseTokenIssuer>();
builder.Services.AddScoped<AccessTokenIssuer>();
builder.Services.AddScoped<RefreshTokenIssuer>();

// ── M16-006: trial state ─────────────────────────────────────────────────
builder.Services.AddScoped<ITrialStateResolver, DatabaseTrialStateResolver>();
builder.Services.AddScoped<TrialSeederService>();
builder.Services.AddScoped<TrialsService>();  // M16-007: on-demand trial activation

// ── M16-008: trial expiry notifier (daily cron) ──────────────────────────
builder.Services.Configure<TrialExpiryNotifierOptions>(
    builder.Configuration.GetSection(TrialExpiryNotifierOptions.Section));
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddHostedService<TrialExpiryNotifier>();

// ── Email queue + processor ───────────────────────────────────────────────
builder.Services.AddSingleton(_ =>
    System.Threading.Channels.Channel.CreateUnbounded<EmailMessage>());
builder.Services.AddSingleton<IEmailQueue, ChannelEmailQueue>();

builder.Services.Configure<EmailQueueProcessorOptions>(
    builder.Configuration.GetSection(EmailQueueProcessorOptions.Section));

// Register the background processor in live mode, AND in Development+fake mode
// so the audit log (registered below) actually receives entries during e2e runs.
// Still never registered in Testing — the BackendFactory wires RecordingEmailQueue instead.
var registerEmailProcessor =
    !builder.Environment.IsEnvironment("Testing") &&
    (sendGridMode == "live" || builder.Environment.IsDevelopment());
if (registerEmailProcessor)
    builder.Services.AddHostedService<EmailQueueProcessor>();

// ── Telemetry background hosts (M18-007) ─────────────────────────────────
// Options wired to configuration so operators can override retention and schedule.
builder.Services.Configure<TelemetryAggregatorOptions>(
    builder.Configuration.GetSection(TelemetryAggregatorOptions.Section));
builder.Services.Configure<TelemetryPurgeOptions>(
    builder.Configuration.GetSection(TelemetryPurgeOptions.Section));
// Never registered in Testing — test hooks trigger TickOnceAsync directly.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<TelemetryAggregatorHost>();  // daily 04:00 UTC aggregate
    builder.Services.AddHostedService<TelemetryPurgeHost>();        // daily 90-day hard-delete
}

// ── Email audit log (Dev+Testing only) ───────────────────────────────────
// Test-only ring-buffer that records successfully-sent emails for assertion
// by the M14-021 convergence spec. Never registered in Production.
if (builder.Environment.IsDevelopment() || builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddSingleton<IRecentlySentEmailLog, InMemoryRecentlySentEmailLog>();

// ── Refresh-token service ─────────────────────────────────────────────────
builder.Services.AddScoped<RefreshTokenService>();

// ── Password reset + email verification (M16-003) ────────────────────────
builder.Services.AddSingleton<IPasswordStrengthChecker, ZxcvbnPasswordStrengthChecker>();
builder.Services.AddScoped<IPasswordResetService, PasswordResetService>();
builder.Services.AddScoped<IEmailVerificationService, EmailVerificationService>();

builder.Services.AddScoped<SubscriptionsService>();
builder.Services.AddScoped<InvitationsService>();
builder.Services.AddScoped<MembersService>();
builder.Services.AddScoped<PrChecksService>();
builder.Services.AddScoped<RoleResolver>();
builder.Services.AddScoped<CustomRolesService>();
builder.Services.AddScoped<CoordinatorService>();
builder.Services.AddScoped<IShardReaper>(sp => sp.GetRequiredService<CoordinatorService>());

// ── SSO / SAML ────────────────────────────────────────────────────────────
builder.Services.Configure<SamlOptions>(builder.Configuration.GetSection(SamlOptions.Section));
// In Testing environment, BackendFactory replaces SamlHandler with FakeSamlHandler.
if (!builder.Environment.IsEnvironment("Testing"))
    builder.Services.AddSingleton<ISamlHandler, SamlHandler>();
builder.Services.AddScoped<SsoService>();
builder.Services.AddScoped<SessionTokenIssuer>();

// ── SSO / OIDC ────────────────────────────────────────────────────────────
builder.Services.Configure<OidcOptions>(builder.Configuration.GetSection(OidcOptions.Section));
builder.Services.AddHttpClient("oidc", c => c.Timeout = TimeSpan.FromSeconds(10));
// In Testing environment, BackendFactory replaces OidcHandler/OidcDiscoveryClient with fakes.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddSingleton<IOidcDiscoveryClient, OidcDiscoveryClient>();
    builder.Services.AddSingleton<IOidcHandler, OidcHandler>();
}
builder.Services.AddScoped<OidcService>();

// ── TeamVault backfill host (M18-009) ────────────────────────────────────
// Encrypts any existing plaintext team_vaults rows on first boot after Migration 1.
// Skipped in Testing — unit tests seed already-encrypted rows via FakeTeamVaultKeyProvider.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<ApiTool.Backend.VaultConfig.TeamVaultBackfillHost>();
}

// ── Scheduler background service ─────────────────────────────────────────
// Only run the scheduler tick in non-Testing environments. Tests exercise
// EnqueueDueAsync directly through SchedulesService.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddHostedService<SchedulerHost>();
    builder.Services.AddHostedService<ShardReaper>();
}

// ── JSON serialization — snake_case ──────────────────────────────────────
builder.Services.ConfigureHttpJsonOptions(opts =>
{
    opts.SerializerOptions.PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower;
    opts.SerializerOptions.Converters.Add(new JsonStringEnumConverter(JsonNamingPolicy.SnakeCaseLower));
    opts.SerializerOptions.DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull;
});

// ── Rate limiting ─────────────────────────────────────────────────────────
// Skip actual limits in Testing environment to keep fixtures deterministic.
if (!builder.Environment.IsEnvironment("Testing"))
{
    builder.Services.AddRateLimiter(options =>
    {
        options.RejectionStatusCode = StatusCodes.Status429TooManyRequests;
        options.AddPolicy("results-ingest", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions
                {
                    PermitLimit = 60,
                    Window = TimeSpan.FromMinutes(1),
                    QueueLimit = 0,
                }));

        options.AddPolicy("checkout-create", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 5, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("subscription-read", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("subscription-update", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 5, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("subscription-delete", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 3, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("portal-create", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.User.FindFirst(System.Security.Claims.ClaimTypes.NameIdentifier)?.Value ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 10, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("invitation-create", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["id"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 20, Window = TimeSpan.FromHours(1), QueueLimit = 0 }));

        options.AddPolicy("invitation-resend", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["inviteId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 3, Window = TimeSpan.FromHours(24), QueueLimit = 0 }));

        options.AddPolicy("sso-login", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("sso-acs", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("oidc-login", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("oidc-callback", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        options.AddPolicy("audit-log-read", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Request.RouteValues["orgId"]?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 30, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));

        // M16-003: per-IP limits for password-reset and email-verification resend endpoints.
        options.AddPolicy("auth-password-reset-request-ip", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Connection.RemoteIpAddress?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 10, Window = TimeSpan.FromHours(24), QueueLimit = 0 }));

        options.AddPolicy("auth-email-verification-resend-ip", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Connection.RemoteIpAddress?.ToString() ?? "anonymous",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 10, Window = TimeSpan.FromHours(24), QueueLimit = 0 }));

        // M18-007: per-install_id rate limit for anonymous telemetry ingest (60/min).
        // install_id is extracted from the request body by TelemetryInstallIdMiddleware
        // and stored in HttpContext.Items["telemetry.install_id"] before this policy fires.
        options.AddPolicy("telemetry-ingest", httpContext =>
            RateLimitPartition.GetFixedWindowLimiter(
                partitionKey: httpContext.Items.TryGetValue("telemetry.install_id", out var id)
                    ? id?.ToString() ?? "unknown"
                    : "unknown",
                factory: _ => new FixedWindowRateLimiterOptions { PermitLimit = 60, Window = TimeSpan.FromMinutes(1), QueueLimit = 0 }));
    });
}
else
{
    // No-op policies so .RequireRateLimiting(...) binds cleanly under the test host.
    builder.Services.AddRateLimiter(options =>
    {
        foreach (var policy in new[]
        {
            "results-ingest", "checkout-create", "subscription-read", "subscription-update",
            "subscription-delete", "portal-create", "invitation-create", "invitation-resend",
            "sso-login", "sso-acs", "oidc-login", "oidc-callback", "audit-log-read",
            "auth-password-reset-request-ip", "auth-email-verification-resend-ip",
            "telemetry-ingest",
        })
        {
            options.AddPolicy(policy, _ => RateLimitPartition.GetNoLimiter("testing"));
        }
    });
}

// ── Antiforgery (required by ASP.NET form-bound endpoints, even when disabled per-endpoint) ──
builder.Services.AddAntiforgery();

// ── Swagger/OpenAPI ───────────────────────────────────────────────────────
builder.Services.AddEndpointsApiExplorer();
builder.Services.AddSwaggerGen(c =>
{
    c.SwaggerDoc("v1", new Microsoft.OpenApi.Models.OpenApiInfo
    {
        Title = "ApiTool Backend",
        Version = "v1",
    });
    c.AddSecurityDefinition("Bearer", new Microsoft.OpenApi.Models.OpenApiSecurityScheme
    {
        Type = Microsoft.OpenApi.Models.SecuritySchemeType.Http,
        Scheme = "bearer",
        BearerFormat = "JWT",
        Description = "Enter a valid JWT bearer token.",
    });
    c.AddSecurityRequirement(new Microsoft.OpenApi.Models.OpenApiSecurityRequirement
    {
        {
            new Microsoft.OpenApi.Models.OpenApiSecurityScheme
            {
                Reference = new Microsoft.OpenApi.Models.OpenApiReference
                {
                    Type = Microsoft.OpenApi.Models.ReferenceType.SecurityScheme,
                    Id = "Bearer",
                },
            },
            Array.Empty<string>()
        },
    });

    // M18-007: document the Idempotency-Key header for the telemetry ingest endpoint.
    c.OperationFilter<ApiTool.Backend.Telemetry.TelemetryIdempotencyKeySwaggerFilter>();
});

// ─────────────────────────────────────────────────────────────────────────
var app = builder.Build();

// ── 413 error shaping ─────────────────────────────────────────────────────
// The request body size limit middleware throws BadHttpRequestException when the
// body exceeds MaxRequestBodySize. Reshape it into our standard ErrorResponse.
app.Use(async (context, next) =>
{
    try
    {
        await next(context);
    }
    catch (Microsoft.AspNetCore.Http.BadHttpRequestException ex)
        when (ex.StatusCode == StatusCodes.Status413RequestEntityTooLarge)
    {
        context.Response.StatusCode = StatusCodes.Status413RequestEntityTooLarge;
        context.Response.ContentType = "application/json";
        var body = JsonSerializer.Serialize(
            new ApiTool.Backend.Organizations.ErrorResponse("payload_too_large", "Request body exceeds the maximum allowed size."),
            new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower });
        await context.Response.WriteAsync(body);
    }
});

// ── Startup: migrations + admin bootstrap (non-Testing only) ─────────────
if (!app.Environment.IsEnvironment("Testing"))
{
    // Dev always auto-migrates for ergonomics; Production requires BACKEND_RUN_MIGRATIONS=1.
    var runMigrations = app.Environment.IsDevelopment()
        || builder.Configuration["BACKEND_RUN_MIGRATIONS"] == "1";

    var migrator = new MigrationRunner(
        app.Services.GetRequiredService<IServiceScopeFactory>(),
        app.Services.GetRequiredService<ILogger<MigrationRunner>>());
    try
    {
        await migrator.RunAsync(runMigrations);
    }
    catch (MigrationFailedException)
    {
        // MigrationRunner has already emitted "migrations: failed".
        Environment.Exit(BootstrapExitCodes.MigrationsFailed);
        return;
    }

    // Bootstrap — only when both env vars are set.
    var (bootCfg, bootErr) = AdminBootstrapConfig.FromConfiguration(builder.Configuration);
    if (bootErr is { } err)
    {
        var msg = err switch
        {
            BootstrapConfigError.EmailMissing    => "bootstrap: BOOTSTRAP_ADMIN_EMAIL is required when BOOTSTRAP_ADMIN_PASSWORD is set",
            BootstrapConfigError.PasswordMissing => "bootstrap: BOOTSTRAP_ADMIN_PASSWORD is required when BOOTSTRAP_ADMIN_EMAIL is set",
            BootstrapConfigError.PasswordTooShort => "bootstrap: password must be >=12 chars",
            BootstrapConfigError.EmailInvalid    => "bootstrap: BOOTSTRAP_ADMIN_EMAIL is not a valid email address",
            _                                    => "bootstrap: invalid configuration",
        };
        app.Logger.LogError("{Message}", msg);
        Environment.Exit(BootstrapExitCodes.BootstrapInvalid);
        return;
    }
    if (bootCfg is not null)
    {
        var boot = new AdminBootstrap(
            app.Services.GetRequiredService<IServiceScopeFactory>(),
            app.Services.GetRequiredService<PasswordHasher>(),
            app.Services.GetRequiredService<ILogger<AdminBootstrap>>(),
            TimeProvider.System);
        await boot.RunAsync(bootCfg);
    }
}

// ── Swagger (Development + Testing) ──────────────────────────────────────
if (app.Environment.IsDevelopment() || app.Environment.IsEnvironment("Testing"))
{
    app.UseSwagger();
    app.UseSwaggerUI();
}

// M18-007: pre-read install_id from telemetry ingest body BEFORE rate-limiter evaluates partition key.
app.UseMiddleware<TelemetryInstallIdMiddleware>();
app.UseRateLimiter();
app.UseAuthentication();
app.UseAuthorization();
app.UseMiddleware<AuditCaptureMiddleware>();
app.UseMiddleware<SchemaGuardMiddleware>();
app.UseAntiforgery();

// ── Endpoints ─────────────────────────────────────────────────────────────
app.MapHealthEndpoints();
app.MapGet("/", () => HttpResults.Redirect("/swagger"));
app.MapAuthEndpoints();
app.MapAuthRefreshEndpoints();            // POST /api/v1/auth/refresh — unified mint (M14-002)
app.MapPasswordResetEndpoints();         // POST /api/v1/auth/password-reset/{request,confirm} (M16-003)
app.MapEmailVerificationEndpoints();    // POST /api/v1/auth/email-verification/{resend,confirm} (M16-003)
app.MapReauthEndpoints();               // POST /api/v1/auth/reauth — drto_ token for destructive ops (M18-005)
app.MapJwksEndpoint();                    // GET /api/v1/.well-known/jwks.json — public JWKS (M14-003)
app.MapInternalRefreshSeedEndpoint(app.Environment); // POST /internal/test/seed-refresh (Dev+Testing only)
app.MapInternalEmailAuditEndpoint(app.Environment);  // GET /internal/test/email-audit (Dev+Testing only) — M14-021
app.MapInternalSeedM14Endpoint(app.Environment);     // POST /internal/test/seed-m14 (Dev+Testing only) — M14-021
app.MapInternalTierGateProbeEndpoint(app.Environment); // POST+GET /internal/test/tier-gate-probe/... (Dev+Testing only) — M16-001
app.MapInternalTrialTickEndpoint(app.Environment);     // POST /internal/test/trial-expiry-tick (Dev+Testing only) — M16-021
app.MapInternalSeedNearExpiryTrialEndpoint(app.Environment); // POST /internal/test/seed-near-expiry-trial (Dev+Testing only) — M16-021
app.MapInternalAuditCleanupTickEndpoint(app.Environment);   // POST /api/v1/internal/test-hooks/run-audit-cleanup (Dev+Testing only) — M18-002
app.MapInternalRunExportBuilderEndpoint(app.Environment);   // POST /api/v1/internal/test-hooks/run-export-builder (Dev+Testing only) — M18-004
app.MapInternalRunDeletionFinalizerEndpoint(app.Environment); // POST /api/v1/internal/test-hooks/run-deletion-finalizer (Dev+Testing only) — M18-005
app.MapTelemetryIngestEndpoint();               // POST /api/v1/telemetry/events (anonymous) — M18-007
app.MapInternalRunTelemetryAggregatorEndpoint(app.Environment); // POST /api/v1/internal/test-hooks/run-telemetry-aggregator (Dev+Testing only) — M18-007
app.MapInternalPurgeTelemetryEventsEndpoint(app.Environment);   // POST /api/v1/internal/test-hooks/purge-telemetry-events (Dev+Testing only) — M18-007
app.MapInternalListTelemetryEventsEndpoint(app.Environment);    // GET /api/v1/internal/test-hooks/list-telemetry-events (Dev+Testing only) — M18-012
app.MapInternalBackdateDeletionEndpoint(app.Environment);       // POST /api/v1/internal/test-hooks/backdate-deletion-request (Dev+Testing only) — M18-012
app.MapInternalMintReauthTokenEndpoint(app.Environment);        // POST /api/v1/internal/test-hooks/mint-reauth-token (Dev+Testing only) — M18-012
app.MapInMemoryObjectStoreEndpoints(app.Environment, app.Services.GetRequiredService<IObjectStore>()); // GET /api/v1/internal/object-store/{key} (Dev+Testing only) — M18-004
app.MapOrganizationsEndpoints();
app.MapResultsEndpoints();
app.MapPrChecksEndpoints();
app.MapSchedulesEndpoints();
app.MapScheduleExecutorEndpoints();   // M16-009
app.MapNotificationsEndpoints();
app.MapSubscriptionsEndpoints();
app.MapInvitationsEndpoints();
app.MapMembersEndpoints();
app.MapSamlEndpoints();
app.MapOidcEndpoints();
app.MapAuditLogEndpoints();
app.MapUserDataExportEndpoints();   // POST/GET /api/v1/users/me/export-requests — M18-004
app.MapUserDeletionEndpoints();     // POST/POST/GET /api/v1/users/me/deletion-requests — M18-005
app.MapCustomRolesEndpoints();
app.MapCoordinatorEndpoints();
app.MapInternalKeysEndpoints();
app.MapInternalGitHubAppEndpoints();  // GET /internal/github-app/jwt-self-test — M14-016
app.MapGithubIntegrationsEndpoints(); // GET /api/v1/integrations/github/install-url + callback — M14-017
app.MapGitLabIntegrationsEndpoints(); // GET/POST/DELETE /api/v1/integrations/gitlab — M16-016
app.MapTrialsEndpoints();          // POST /api/v1/trials/{feature} — M16-007
app.MapVaultConfigEndpoints();     // GET/PUT/DELETE /api/v1/organizations/{orgId}/vault-config — M16-017
app.MapDashboardEndpoints();       // GET /api/v1/organizations/{orgId}/results/{stats,failures} — M16-019
app.MapStripeWebhookEndpoint();   // POST /webhooks/stripe — M14-011
app.MapGithubWebhookEndpoint();   // POST /webhooks/github — M14-019
app.MapGitLabWebhookEndpoint();   // POST /webhooks/gitlab — M16-015

app.Run();

/// <summary>
/// Entry point type exposed for WebApplicationFactory in tests.
/// </summary>
public partial class Program;
