using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Tests the canonical template inventory: slug count, spec alignment, and file-pair presence.
/// Filter: FullyQualifiedName~EmailTemplateInventory
/// </summary>
public sealed class EmailTemplateInventoryTests
{
    [Fact]
    public void Inventory_has_ten_slugs()
        => EmailTemplateInventory.Slugs.Should().HaveCount(10,
            because: "M18-005 adds account_deletion_initiated + account_deletion_completed");

    [Fact]
    public void Inventory_matches_spec()
    {
        // M14 six (:8926-8933) plus M16 two (:8584-8589) plus M18-005 two.
        var expected = new[]
        {
            "email_verification",
            "auth_device_code",
            "billing_receipt",
            "billing_payment_failed",
            "billing_subscription_canceled",
            "account_security_alert",
            "password_reset",
            "trial_expiring",
            // M18-005 additions (v4-5):
            "account_deletion_initiated",
            "account_deletion_completed",
        };
        EmailTemplateInventory.Slugs.Should().BeEquivalentTo(expected,
            because: "the inventory must match the M14+M16+M18-005 spec entries exactly");
    }

    [Fact]
    public void Inventory_contains_account_deletion_initiated() =>
        EmailTemplateInventory.Slugs.Should().Contain("account_deletion_initiated",
            because: "M18-005 adds the account_deletion_initiated template");

    [Fact]
    public void Inventory_contains_account_deletion_completed() =>
        EmailTemplateInventory.Slugs.Should().Contain("account_deletion_completed",
            because: "M18-005 adds the account_deletion_completed template");

    // Forbidden_templates_are_absent: REMOVED — password_reset and trial_expiring now ship in M16-002.

    [Fact]
    public void Each_slug_has_both_mjml_and_json_files()
    {
        var root = Path.Combine(GetRepoRoot(), "templates", "email");
        foreach (var slug in EmailTemplateInventory.Slugs)
        {
            File.Exists(Path.Combine(root, $"{slug}.mjml")).Should().BeTrue(
                because: $"inventory requires {slug}.mjml");
            File.Exists(Path.Combine(root, $"{slug}.json")).Should().BeTrue(
                because: $"inventory requires {slug}.json");
        }
    }

    internal static string GetRepoRoot()
    {
        // Walk up from AppContext.BaseDirectory until we find the templates/email directory.
        var dir = new DirectoryInfo(AppContext.BaseDirectory);
        while (dir is not null)
        {
            if (Directory.Exists(Path.Combine(dir.FullName, "templates", "email")))
                return dir.FullName;
            dir = dir.Parent;
        }

        throw new InvalidOperationException(
            "Could not locate repo root containing templates/email from " + AppContext.BaseDirectory);
    }
}
