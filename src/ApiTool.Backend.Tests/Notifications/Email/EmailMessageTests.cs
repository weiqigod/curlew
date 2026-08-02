// Tests for EmailMessage and IEmailQueue — Step 1 of M14-002.
// Refs docs/SPECIFICATION.md:8924-8953 (account_security_alert template variables).
using System.Threading.Channels;
using ApiTool.Backend.Notifications.Email;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class EmailMessageTests
{
    [Fact]
    public void account_security_alert_required_variables_match_M14_014_manifest()
    {
        // Spec docs/SPECIFICATION.md:8933 — variables are
        //   first_name, event_time, event_ip, relogin_url.
        var msg = new EmailMessage(
            To: "user@example.com",
            TemplateSlug: "account_security_alert",
            Variables: new Dictionary<string, string>
            {
                ["first_name"]  = "Alex",
                ["event_time"]  = "2026-05-04T12:34:56Z",
                ["event_ip"]    = "203.0.113.42",
                ["relogin_url"] = "https://app.apitool.dev/login",
            },
            EnqueuedAt: DateTimeOffset.UtcNow);

        msg.Variables.Keys.Should().BeEquivalentTo(
            new[] { "first_name", "event_time", "event_ip", "relogin_url" });
    }

    [Fact]
    public async Task ChannelEmailQueue_writes_message_to_channel()
    {
        var ch = Channel.CreateUnbounded<EmailMessage>();
        var q = new ChannelEmailQueue(ch);
        await q.EnqueueAsync(new EmailMessage(
            "to@x.com",
            "account_security_alert",
            new Dictionary<string, string>(),
            DateTimeOffset.UtcNow));
        ch.Reader.TryRead(out var got).Should().BeTrue();
        got!.To.Should().Be("to@x.com");
    }

    [Fact]
    public async Task RecordingEmailQueue_captures_all_enqueued_messages()
    {
        var q = new RecordingEmailQueue();
        var msg1 = new EmailMessage("a@b.com", "slug1", new Dictionary<string, string>(), DateTimeOffset.UtcNow);
        var msg2 = new EmailMessage("c@d.com", "slug2", new Dictionary<string, string>(), DateTimeOffset.UtcNow);
        await q.EnqueueAsync(msg1);
        await q.EnqueueAsync(msg2);
        q.Messages.Should().HaveCount(2);
        q.Messages[0].To.Should().Be("a@b.com");
        q.Messages[1].To.Should().Be("c@d.com");
    }
}
