// Tests for GitLabOptions configuration binding.
using ApiTool.Backend.GitLab;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests.GitLab;

/// <summary>
/// Tests that <see cref="GitLabOptions"/> binds correctly from IConfiguration.
/// </summary>
public sealed class GitLabOptionsBindingTests
{
    [Fact]
    public void Binds_file_mode_from_config()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:GitLab:KeyProvider:Mode"] = "file",
                ["ApiTool:GitLab:KeyProvider:File:KekPath"] = "Keys/gitlab-kek.bin",
            }).Build();

        var opts = cfg.GetSection(GitLabOptions.Section).Get<GitLabOptions>()!;
        opts.KeyProvider.Mode.Should().Be("file");
        opts.KeyProvider.File.KekPath.Should().Be("Keys/gitlab-kek.bin");
    }

    [Fact]
    public void Binds_kms_mode_from_config()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:GitLab:KeyProvider:Mode"] = "kms",
                ["ApiTool:GitLab:KeyProvider:Kms:KmsKeyId"] = "projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1",
            }).Build();

        var opts = cfg.GetSection(GitLabOptions.Section).Get<GitLabOptions>()!;
        opts.KeyProvider.Mode.Should().Be("kms");
        opts.KeyProvider.Kms.KmsKeyId.Should().Be("projects/p/locations/global/keyRings/kr/cryptoKeys/k/cryptoKeyVersions/1");
    }

    [Fact]
    public void Poster_AllowHttp_DefaultsFalse()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                // Need at least one key so Get<GitLabOptions>() returns a non-null instance.
                ["ApiTool:GitLab:KeyProvider:Mode"] = "file",
            }).Build();

        var opts = cfg.GetSection(GitLabOptions.Section).Get<GitLabOptions>()!;
        opts.Poster.AllowHttp.Should().BeFalse();
    }

    [Fact]
    public void Poster_AllowHttp_BindsToTrue_WhenSet()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:GitLab:Poster:AllowHttp"] = "true",
            }).Build();

        var opts = cfg.GetSection(GitLabOptions.Section).Get<GitLabOptions>()!;
        opts.Poster.AllowHttp.Should().BeTrue();
    }
}
