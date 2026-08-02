using ApiTool.Backend.GitHub;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests.GitHub;

/// <summary>
/// Tests that GitHubAppOptions can be bound from Microsoft.Extensions.Configuration.
/// </summary>
public sealed class GitHubAppOptionsBindingTests
{
    [Fact]
    public void Binds_GITHUB_APP_envvars_to_options()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:GitHubApp:AppId"] = "12345",
                ["ApiTool:GitHubApp:Slug"] = "apitool-checks-test",
                ["ApiTool:GitHubApp:KeyProvider:Mode"] = "file",
                ["ApiTool:GitHubApp:KeyProvider:File:Path"] = "./Keys/github-app/test.pem",
            }).Build();

        var opts = cfg.GetSection(GitHubAppOptions.Section).Get<GitHubAppOptions>()!;

        opts.AppId.Should().Be(12345);
        opts.Slug.Should().Be("apitool-checks-test");
        opts.KeyProvider.Mode.Should().Be("file");
        opts.KeyProvider.File.Path.Should().Be("./Keys/github-app/test.pem");
    }
}
