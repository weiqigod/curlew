using ApiTool.Backend;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Tests;

/// <summary>Verifies that <see cref="AppOptions"/> binds correctly from configuration.</summary>
public sealed class AppOptionsTests
{
    [Fact]
    public void Defaults_to_empty_web_app_url()
    {
        new AppOptions().WebAppUrl.Should().BeEmpty();
    }

    [Fact]
    public void Bound_from_configuration()
    {
        var cfg = new ConfigurationBuilder()
            .AddInMemoryCollection(new Dictionary<string, string?>
            {
                ["ApiTool:App:WebAppUrl"] = "https://app.example.test",
            }).Build();
        var opts = new AppOptions();
        cfg.GetSection(AppOptions.Section).Bind(opts);
        opts.WebAppUrl.Should().Be("https://app.example.test");
    }
}
