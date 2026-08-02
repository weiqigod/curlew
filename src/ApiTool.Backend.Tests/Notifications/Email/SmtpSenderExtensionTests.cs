using ApiTool.Backend.Notifications;
using FluentAssertions;
using Microsoft.Extensions.Logging;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class SmtpSenderExtensionTests
{
    [Fact]
    public async Task NoopSmtpSender_SendTemplateAsync_logs_and_returns()
    {
        using var lf = LoggerFactory.Create(b => b.AddConsole());
        var sender = new NoopSmtpSender(lf.CreateLogger<NoopSmtpSender>());
        // Should not throw.
        await sender.SendTemplateAsync("to@x.com", "slug", new Dictionary<string, string>(), default);
    }

    [Fact]
    public async Task RecordingSmtpSender_captures_template_sends()
    {
        var sender = new RecordingSmtpSender();
        await sender.SendTemplateAsync("to@x.com", "slug",
            new Dictionary<string, string> { ["a"] = "b" }, default);
        sender.TemplateSends.Should().ContainSingle()
            .Which.Should().BeEquivalentTo((
                To: "to@x.com",
                Slug: "slug",
                Variables: new Dictionary<string, string> { ["a"] = "b" }));
    }

    [Fact]
    public async Task RecordingSmtpSender_throws_scripted_exception()
    {
        var sender = new RecordingSmtpSender();
        sender.SendResults.Enqueue(new InvalidOperationException("scripted error"));

        await Assert.ThrowsAsync<InvalidOperationException>(
            () => sender.SendTemplateAsync("to@x.com", "slug",
                new Dictionary<string, string>(), default));
    }

    [Fact]
    public async Task FakeSmtpSender_SendTemplateAsync_does_not_throw()
    {
        var sender = new ApiTool.Backend.Tests.Notifications.FakeSmtpSender();
        await sender.SendTemplateAsync("to@x.com", "slug", new Dictionary<string, string>(), default);
    }
}
