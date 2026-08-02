using ApiTool.Backend.Auth;
using ApiTool.Backend.Bootstrap;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Data.Sqlite;
using Microsoft.EntityFrameworkCore;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests.Bootstrap;

/// <summary>Unit tests for the admin bootstrap startup task.</summary>
public sealed class AdminBootstrapTests
{
    private static IConfiguration Build(Dictionary<string, string?> values) =>
        new ConfigurationBuilder().AddInMemoryCollection(values).Build();

    [Fact]
    public void FromConfiguration_both_unset_returns_disabled()
    {
        var cfg = Build(new());
        var (c, err) = AdminBootstrapConfig.FromConfiguration(cfg);
        c.Should().BeNull();
        err.Should().BeNull();
    }

    [Theory]
    [InlineData("", "abcdefghijkl", BootstrapConfigError.EmailMissing)]
    [InlineData("admin@example.com", "", BootstrapConfigError.PasswordMissing)]
    [InlineData("admin@example.com", "tooshort", BootstrapConfigError.PasswordTooShort)]
    [InlineData("not-an-email", "abcdefghijklmnop", BootstrapConfigError.EmailInvalid)]
    public void FromConfiguration_partial_or_invalid_returns_error(
        string email, string password, BootstrapConfigError expected)
    {
        var cfg = Build(new Dictionary<string, string?>
        {
            ["BOOTSTRAP_ADMIN_EMAIL"] = email,
            ["BOOTSTRAP_ADMIN_PASSWORD"] = password,
        });
        var (_, err) = AdminBootstrapConfig.FromConfiguration(cfg);
        err.Should().Be(expected);
    }

    [Fact]
    public void FromConfiguration_valid_pair_returns_config_with_lowercased_email()
    {
        var cfg = Build(new Dictionary<string, string?>
        {
            ["BOOTSTRAP_ADMIN_EMAIL"] = "Admin@Example.COM",
            ["BOOTSTRAP_ADMIN_PASSWORD"] = "ChangeMe!Password",
        });
        var (config, err) = AdminBootstrapConfig.FromConfiguration(cfg);
        err.Should().BeNull();
        config.Should().NotBeNull();
        config!.Email.Should().Be("admin@example.com");
        config.Password.Should().Be("ChangeMe!Password");
    }

    [Fact]
    public async Task RunAsync_empty_db_creates_admin_user_and_default_org()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        var boot = new AdminBootstrap(
            new SingletonScopeFactory(scope.Db),
            new PasswordHasher(),
            new TestLogger<AdminBootstrap>(),
            TimeProvider.System);
        var created = await boot.RunAsync(new AdminBootstrapConfig("admin@example.com", "ChangeMe!Password"));
        created.Should().BeTrue();

        var user = await scope.Db.Users.SingleAsync();
        user.IsAdmin.Should().BeTrue();
        user.PasswordHash.Should().NotBeNullOrEmpty();
        (await scope.Db.Organizations.CountAsync()).Should().Be(1);
        (await scope.Db.OrganizationMembers.SingleAsync()).Role.Should().Be(OrgRole.Owner);
        (await scope.Db.Subscriptions.SingleAsync()).Tier.Should().Be(SubscriptionTier.Enterprise);
        (await scope.Db.Trials.CountAsync()).Should().Be(TrialFeatures.All.Count,
            because: "AdminBootstrap must seed one full_initial row per trialable feature");
    }

    [Fact]
    public async Task RunAsync_existing_user_is_idempotent()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();
        scope.Db.Users.Add(new User
        {
            Id = Guid.NewGuid(),
            Email = "admin@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        var sink = new TestLogger<AdminBootstrap>();
        var boot = new AdminBootstrap(
            new SingletonScopeFactory(scope.Db),
            new PasswordHasher(),
            sink,
            TimeProvider.System);
        var created = await boot.RunAsync(new AdminBootstrapConfig("admin@example.com", "ChangeMe!Password"));
        created.Should().BeFalse();
        sink.Entries.Should().ContainSingle(e => e.Message.Contains("already exists, skipping"));
        (await scope.Db.Users.CountAsync()).Should().Be(1);
    }

    [Fact]
    public async Task RunAsync_email_is_lowercased_before_storing()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        var boot = new AdminBootstrap(
            new SingletonScopeFactory(scope.Db),
            new PasswordHasher(),
            new TestLogger<AdminBootstrap>(),
            TimeProvider.System);
        await boot.RunAsync(new AdminBootstrapConfig("Admin@Example.COM", "ChangeMe!Password"));

        var user = await scope.Db.Users.SingleAsync();
        user.Email.Should().Be("admin@example.com");
    }

    [Fact]
    public async Task RunAsync_rolls_back_transaction_when_save_fails()
    {
        // Open a real SQLite connection so we can create the schema with a normal context
        // and then re-open the same DB with the failing interceptor.
        var conn = new SqliteConnection("DataSource=:memory:");
        conn.Open();
        await using var conn_ = conn; // dispose at end of test

        // Phase 1: create schema using a clean context on the same connection.
        await using var schemaScope = TestDb.CreateOpenFromConnection(conn);
        await schemaScope.Db.Database.EnsureCreatedAsync();

        // Phase 2: create the failing context on the same connection.
        var failingFactory = FailOnSaveScopeFactory.Create(conn);
        var boot = new AdminBootstrap(
            failingFactory,
            new PasswordHasher(),
            new TestLogger<AdminBootstrap>(),
            TimeProvider.System);

        // Bootstrap should throw because SaveChangesAsync fails.
        var act = async () => await boot.RunAsync(new AdminBootstrapConfig("admin@example.com", "ChangeMe!Password"));
        await act.Should().ThrowAsync<Exception>();

        // Verify no partial data was committed — user table must still be empty.
        var count = await schemaScope.Db.Users.CountAsync();
        count.Should().Be(0, "transaction rollback must undo any partial writes");
    }

    [Fact]
    public async Task RunAsync_slug_collision_uses_numbered_suffix()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.EnsureCreatedAsync();

        // Pre-seed an org with slug "default" to force the collision path.
        var existingOwnerId = Guid.NewGuid();
        scope.Db.Users.Add(new User
        {
            Id        = existingOwnerId,
            Email     = "existing@example.com",
            CreatedAt = DateTime.UtcNow,
        });
        scope.Db.Organizations.Add(new Organization
        {
            Id        = Guid.NewGuid(),
            Name      = "Existing Org",
            Slug      = "default",
            OwnerId   = existingOwnerId,
            Status    = OrgStatus.Active,
            CreatedAt = DateTime.UtcNow,
            UpdatedAt = DateTime.UtcNow,
        });
        await scope.Db.SaveChangesAsync();

        var boot = new AdminBootstrap(
            new SingletonScopeFactory(scope.Db),
            new PasswordHasher(),
            new TestLogger<AdminBootstrap>(),
            TimeProvider.System);
        var created = await boot.RunAsync(new AdminBootstrapConfig("admin@example.com", "ChangeMe!Password"));
        created.Should().BeTrue();

        // The bootstrap org must get slug "default-1" since "default" is taken.
        var bootstrapOrg = await scope.Db.Organizations.SingleAsync(o => o.OwnerId != existingOwnerId);
        bootstrapOrg.Slug.Should().Be("default-1");
    }

    [Fact]
    public async Task RunAsync_seeds_full_initial_trial_rows_for_admin_user()
    {
        await using var scope = TestDb.CreateOpen();
        await scope.Db.Database.MigrateAsync();

        var boot = new AdminBootstrap(
            new SingletonScopeFactory(scope.Db),
            new PasswordHasher(),
            new TestLogger<AdminBootstrap>(),
            TimeProvider.System);
        await boot.RunAsync(new AdminBootstrapConfig("admin@example.com", "ChangeMe!Password"));

        var user = await scope.Db.Users.SingleAsync();
        var trials = await scope.Db.Trials.Where(t => t.UserId == user.Id).ToListAsync();
        trials.Should().HaveCount(TrialFeatures.All.Count,
            because: "AdminBootstrap must seed one full_initial row per trialable feature");
        trials.Should().OnlyContain(t => t.Kind == TrialKind.FullInitial);
        trials.Select(t => t.Feature).Should().BeEquivalentTo(TrialFeatures.All);
    }
}
