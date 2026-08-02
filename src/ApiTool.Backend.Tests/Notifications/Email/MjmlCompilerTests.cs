using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class MjmlCompilerTests
{
    [Fact]
    public void Compile_renders_mjml_to_html()
    {
        var html = new MjmlNetCompiler().Compile(
            "<mjml><mj-body><mj-section><mj-column><mj-text>Hi</mj-text></mj-column></mj-section></mj-body></mjml>");
        html.Should().Contain("Hi");
        html.Should().Contain("<!doctype html>", because: "MJML emits a full HTML doc");
    }

    [Fact]
    public void Compile_invalid_mjml_throws_InvalidOperationException()
        => Assert.Throws<InvalidOperationException>(
            () => new MjmlNetCompiler().Compile("not mjml at all {{{{"));

    [Fact]
    public void IMjmlCompiler_seam_is_satisfied_by_MjmlNetCompiler()
    {
        IMjmlCompiler compiler = new MjmlNetCompiler();
        var html = compiler.Compile(
            "<mjml><mj-body><mj-section><mj-column><mj-text>Test</mj-text></mj-column></mj-section></mj-body></mjml>");
        html.Should().NotBeEmpty();
    }
}
