using System.Text.RegularExpressions;

namespace ApiTool.Backend.Tests.Compliance;

/// <summary>
/// Asserts structural invariants of <c>docs/security/data-classification-matrix.md</c>
/// and its consistency with <c>docs/security/data-inventory.md</c>.
///
/// The data-classification matrix is the broader superset: it covers all 36 schema-appendix
/// tables. The GDPR data inventory covers only the 14 user-attributable tables. The enforced
/// direction is: every table in the inventory must appear in the matrix (inventory ⊆ matrix).
/// The reverse is not required because the matrix's non-user-attributable tables carry no GDPR
/// disposition.
///
/// Marked [Trait("Category", "DocConsistency")] so it can be filtered out of fast inner-loop
/// test runs without removing it from CI.
/// </summary>
[Trait("Category", "DocConsistency")]
public class DataClassificationMatrixDocTests
{
    private static readonly string MatrixPath = Path.Combine(
        SolutionRoot(), "docs", "security", "data-classification-matrix.md");

    private static readonly string InventoryPath = Path.Combine(
        SolutionRoot(), "docs", "security", "data-inventory.md");

    /// <summary>
    /// All 36 schema-appendix tables that must appear in the classification matrix.
    /// Sourced from SPECIFICATION.md lines 10500–11258. When a new migration adds a table,
    /// add it here AND in the matrix — the test names the missing entry.
    /// </summary>
    private static readonly IReadOnlyList<string> SchemaAppendixTables =
    [
        // Auth
        "auth_audit_log",
        "deletion_reauth_tokens",
        "device_authorization_codes",
        "devices",
        "email_verification_tokens",
        "magic_link_tokens",
        "oauth_state_tokens",
        "password_reset_tokens",
        "refresh_tokens",
        "sessions",
        "service_tokens",
        "signing_keys",
        "user_audit_log",
        "users",
        // Billing
        "invoices",
        "payment_methods",
        "stripe_webhook_events",
        "subscription_audit_log",
        "subscriptions",
        // Integrations
        "github_installations",
        "github_webhook_events",
        "gitlab_installations",
        "gitlab_webhook_events",
        "pr_checks",
        // Workspace
        "coordinator_jobs",
        "license_validations",
        "notification_rules",
        "organization_audit_log",
        "organization_custom_roles",
        "organization_invitations",
        "organization_members",
        "organizations",
        "scheduled_runs",
        "schedules",
        "team_vaults",
        "trials",
    ];

    [Fact]
    public void Matrix_doc_exists() =>
        File.Exists(MatrixPath).Should().BeTrue($"expected file to exist at: {MatrixPath}");

    [Fact]
    public void Matrix_carries_four_tier_classification()
    {
        var text = File.ReadAllText(MatrixPath);
        text.Should().Contain("Public",
            "the matrix must define the Public classification tier");
        text.Should().Contain("Internal",
            "the matrix must define the Internal classification tier");
        text.Should().Contain("Confidential",
            "the matrix must define the Confidential classification tier");
        text.Should().Contain("Restricted",
            "the matrix must define the Restricted classification tier");
    }

    [Fact]
    public void Matrix_references_envelope_encryption_for_team_vaults_and_env_vars()
    {
        var text = File.ReadAllText(MatrixPath);
        text.Should().Contain("team_vaults",
            "the matrix must reference team_vaults (M18-009 envelope encryption)");
        text.Should().Contain("schedules.env_vars",
            "the matrix must reference schedules.env_vars (M18-009 envelope encryption)");
        text.Should().Contain("envelope",
            "the matrix must reference envelope encryption (M18-009 posture)");
    }

    [Fact]
    public void Every_inventory_table_appears_in_matrix()
    {
        var inventoryText = File.ReadAllText(InventoryPath);
        var matrixText = File.ReadAllText(MatrixPath);

        var inventoryNames = ParseBacktickTableNames(inventoryText);
        var matrixNames = ParseBacktickTableNames(matrixText);

        var missing = inventoryNames.Except(matrixNames, StringComparer.Ordinal).OrderBy(s => s).ToList();

        missing.Should().BeEmpty(
            $"every table in data-inventory.md must appear in data-classification-matrix.md " +
            $"(inventory ⊆ matrix). Missing: {string.Join(", ", missing)}");
    }

    [Fact]
    public void Matrix_lists_all_36_schema_appendix_tables()
    {
        var text = File.ReadAllText(MatrixPath);
        var matrixNames = ParseBacktickTableNames(text);

        var missing = SchemaAppendixTables
            .Except(matrixNames, StringComparer.Ordinal)
            .OrderBy(s => s)
            .ToList();

        missing.Should().BeEmpty(
            $"every schema-appendix table must be classified in data-classification-matrix.md. " +
            $"Missing: {string.Join(", ", missing)}");
    }

    [Fact]
    public void ComplianceMd_links_to_matrix()
    {
        var compliancePath = Path.Combine(SolutionRoot(), "docs", "COMPLIANCE.md");
        File.ReadAllText(compliancePath).Should().Contain("data-classification-matrix.md",
            "docs/COMPLIANCE.md must cross-link to the data classification matrix (M18-010 DoD)");
    }

    /// <summary>
    /// Parses backtick-wrapped table names from the first column of Markdown table rows.
    /// Matches rows of the form <c>| `table_name` | ... |</c>. Tolerant of leading/trailing
    /// whitespace around pipes and backticks.
    /// </summary>
    private static IReadOnlySet<string> ParseBacktickTableNames(string md)
    {
        var rowRegex = new Regex(@"^\|\s*`([^`]+)`\s*\|", RegexOptions.Compiled | RegexOptions.Multiline);
        var results = new HashSet<string>(StringComparer.Ordinal);

        foreach (Match m in rowRegex.Matches(md))
            results.Add(m.Groups[1].Value);

        return results;
    }

    private static string SolutionRoot()
    {
        var dir = AppContext.BaseDirectory;
        while (dir is not null)
        {
            if (File.Exists(Path.Combine(dir, "ApiTool.Backend.sln")))
                return dir;
            dir = Path.GetDirectoryName(dir);
        }

        throw new InvalidOperationException(
            "Could not locate solution root (ApiTool.Backend.sln) from " + AppContext.BaseDirectory);
    }
}
