using ApiTool.Backend.Licensing.Trials;

namespace ApiTool.Backend.Tests.Licensing.Trials;

public sealed class TrialFeaturesTests
{
    [Fact]
    public void All_contains_spec_named_features_only()
    {
        TrialFeatures.All.Should().Contain("vault_provider_profiles")
            .And.Contain("shared_vault_templates")
            .And.Contain("schedules");
        TrialFeatures.All.Should().OnlyHaveUniqueItems(
            because: "duplicate slugs would violate UNIQUE(user_id, feature)");
    }

    [Fact]
    public void All_slugs_are_lowercase_snake_case_and_within_64_chars()
    {
        // Matches the trials.feature column max length (HasMaxLength(64)).
        foreach (var slug in TrialFeatures.All)
        {
            slug.Length.Should().BeLessOrEqualTo(64);
            slug.Should().MatchRegex("^[a-z][a-z0-9_]*$");
        }
    }
}
