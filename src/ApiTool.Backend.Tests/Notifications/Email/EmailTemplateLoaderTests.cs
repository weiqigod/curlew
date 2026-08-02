using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class EmailTemplateLoaderTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), $"tmpl_{Guid.NewGuid():N}");

    public EmailTemplateLoaderTests() => Directory.CreateDirectory(_root);

    public void Dispose()
    {
        try { Directory.Delete(_root, true); } catch { /* ignore cleanup failures */ }
    }

    [Fact]
    public void LoadMjml_returns_file_contents()
    {
        File.WriteAllText(Path.Combine(_root, "x.mjml"), "<mjml/>");
        new EmailTemplateLoader(_root).LoadMjml("x").Should().Be("<mjml/>");
    }

    [Fact]
    public void LoadMjml_missing_file_throws_FileNotFoundException()
        => Assert.Throws<FileNotFoundException>(
            () => new EmailTemplateLoader(_root).LoadMjml("nope"));

    [Fact]
    public void LoadManifest_round_trips_schema()
    {
        File.WriteAllText(Path.Combine(_root, "x.json"), """
            {"slug":"x","subject":"S","variables":{"a":"string"},"test_data":{"a":"hi"}}
            """);
        var m = new EmailTemplateLoader(_root).LoadManifest("x");
        m.Slug.Should().Be("x");
        m.Subject.Should().Be("S");
        m.Variables.Should().ContainKey("a");
        m.TestData["a"].Should().Be("hi");
    }

    [Fact]
    public void LoadManifest_missing_file_throws_FileNotFoundException()
        => Assert.Throws<FileNotFoundException>(
            () => new EmailTemplateLoader(_root).LoadManifest("nope"));

    [Fact]
    public void LoadManifest_invalid_json_throws_InvalidDataException()
    {
        File.WriteAllText(Path.Combine(_root, "bad.json"), "not json");
        Assert.Throws<InvalidDataException>(
            () => new EmailTemplateLoader(_root).LoadManifest("bad"));
    }
}
