using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class SendGridOptionsTests
{
    [Fact]
    public void Templates_lookup_is_case_insensitive()
    {
        var opts = new SendGridOptions();
        opts.Templates["email_verification"] = "d-abc";
        opts.Templates.TryGetValue("EMAIL_VERIFICATION", out var got).Should().BeTrue();
        got.Should().Be("d-abc");
    }

    [Fact]
    public void Section_constant_matches_spec_path()
        => SendGridOptions.Section.Should().Be("ApiTool:SendGrid");

    [Fact]
    public void Default_mode_is_fake()
        => new SendGridOptions().Mode.Should().Be("fake");

    [Fact]
    public void Default_from_email_is_set()
        => new SendGridOptions().FromEmail.Should().Be("noreply@apitool.dev");
}
