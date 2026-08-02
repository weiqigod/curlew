using ApiTool.Backend.Notifications;
using ApiTool.Backend.Notifications.Email;
using FluentAssertions;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;
using SendGrid;
using SendGrid.Helpers.Mail;

namespace ApiTool.Backend.Tests.Notifications;

public sealed class SendGridSmtpSenderTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), $"sg_{Guid.NewGuid():N}");

    public SendGridSmtpSenderTests()
    {
        Directory.CreateDirectory(_root);
        // Write a minimal manifest for slug "x" with one declared variable "a".
        File.WriteAllText(Path.Combine(_root, "x.json"), """
            {"slug":"x","subject":"S","variables":{"a":"string"},"test_data":{"a":"hi"}}
            """);
    }

    public void Dispose()
    {
        try { Directory.Delete(_root, true); } catch { /* ignore */ }
    }

    private (SendGridSmtpSender Sender, StubSendGridClient Client) MakeSender(
        Func<SendGridMessage, Response> reply,
        Dictionary<string, string>? templates = null)
    {
        var opts = new SendGridOptions
        {
            Mode = "live",
            ApiKey = "SG.fake",
            FromEmail = "from@test.com",
            FromName = "Test",
        };
        if (templates is not null)
        {
            foreach (var (k, v) in templates)
                opts.Templates[k] = v;
        }

        var client = new StubSendGridClient(reply);
        var loader = new EmailTemplateLoader(_root);
        var sender = new SendGridSmtpSender(
            Options.Create(opts), loader, NullLogger<SendGridSmtpSender>.Instance, client);
        return (sender, client);
    }

    private static Response OkResponse()
        => new(System.Net.HttpStatusCode.OK, null, null);

    private static Response MakeResponse(int statusCode)
        => new((System.Net.HttpStatusCode)statusCode, null, null);

    [Fact]
    public async Task SendTemplateAsync_unknown_slug_throws_EmailTemplateNotFoundException()
    {
        var (sender, _) = MakeSender(_ => OkResponse(), templates: []);
        await Assert.ThrowsAsync<EmailTemplateNotFoundException>(
            () => sender.SendTemplateAsync("to@x.com", "no_such_slug", new Dictionary<string, string>(), default));
    }

    [Fact]
    public async Task SendTemplateAsync_unknown_variable_throws_EmailTemplateVariableUnknownException()
    {
        var (sender, _) = MakeSender(_ => OkResponse(), templates: new() { ["x"] = "d-1" });
        var ex = await Assert.ThrowsAsync<EmailTemplateVariableUnknownException>(
            () => sender.SendTemplateAsync("to@x.com", "x",
                new Dictionary<string, string> { ["unknown"] = "v" }, default));
        ex.UnknownVariables.Should().Contain("unknown");
    }

    [Fact]
    public async Task SendTemplateAsync_happy_path_posts_template_id()
    {
        SendGridMessage? captured = null;
        var (sender, _) = MakeSender(m => { captured = m; return OkResponse(); },
            templates: new() { ["x"] = "d-1" });
        await sender.SendTemplateAsync("to@x.com", "x",
            new Dictionary<string, string> { ["a"] = "b" }, default);
        captured.Should().NotBeNull();
        captured!.TemplateId.Should().Be("d-1");
    }

    [Fact]
    public async Task SendTemplateAsync_5xx_raises_SendGridUnavailableException()
    {
        var (sender, _) = MakeSender(_ => MakeResponse(503), templates: new() { ["x"] = "d-1" });
        await Assert.ThrowsAsync<SendGridSmtpSender.SendGridUnavailableException>(
            () => sender.SendTemplateAsync("to@x.com", "x",
                new Dictionary<string, string>(), default));
    }

    [Fact]
    public async Task SendTemplateAsync_429_raises_SendGridRateLimitedException()
    {
        var (sender, _) = MakeSender(_ => MakeResponse(429), templates: new() { ["x"] = "d-1" });
        await Assert.ThrowsAsync<SendGridSmtpSender.SendGridRateLimitedException>(
            () => sender.SendTemplateAsync("to@x.com", "x",
                new Dictionary<string, string>(), default));
    }

    [Fact]
    public async Task SendTemplateAsync_4xx_raises_SendGridPermanentException()
    {
        var (sender, _) = MakeSender(_ => MakeResponse(400), templates: new() { ["x"] = "d-1" });
        await Assert.ThrowsAsync<SendGridSmtpSender.SendGridPermanentException>(
            () => sender.SendTemplateAsync("to@x.com", "x",
                new Dictionary<string, string>(), default));
    }

    [Fact]
    public async Task SendTemplateAsync_resolves_slug_case_insensitively()
    {
        var (sender, _) = MakeSender(_ => OkResponse(),
            templates: new() { ["X"] = "d-ci" });
        // lowercase slug lookup should match uppercase key
        await sender.SendTemplateAsync("to@x.com", "x",
            new Dictionary<string, string>(), default);
    }

    [Fact]
    public async Task SendTemplateAsync_empty_variables_is_accepted()
    {
        var (sender, _) = MakeSender(_ => OkResponse(), templates: new() { ["x"] = "d-1" });
        // "x" manifest declares "a" as variable but we send no variables — allowed (not all vars required)
        await sender.SendTemplateAsync("to@x.com", "x",
            new Dictionary<string, string>(), default);
    }

    [Fact]
    public async Task SendAsync_happy_path_sends_plain_text()
    {
        var (sender, client) = MakeSender(_ => OkResponse());
        await sender.SendAsync("to@x.com", "Subject", "Body text", default);
        client.CallCount.Should().Be(1);
    }
}

/// <summary>Stub <see cref="ISendGridClient"/> that scripts canned responses.</summary>
internal sealed class StubSendGridClient(Func<SendGridMessage, Response> reply) : ISendGridClient
{
    private int _calls;
    public int CallCount => _calls;

    public Task<Response> SendEmailAsync(SendGridMessage msg, CancellationToken ct = default)
    {
        Interlocked.Increment(ref _calls);
        return Task.FromResult(reply(msg));
    }

    // Unused members — stubs to satisfy interface.
    public string UrlPath { get; set; } = string.Empty;
    public string Version { get; set; } = string.Empty;
    public string MediaType { get; set; } = string.Empty;

    public System.Net.Http.Headers.AuthenticationHeaderValue AddAuthorization(KeyValuePair<string, string> header)
        => new("Bearer", "fake");

    public Task<Response> RequestAsync(BaseClient.Method method, string requestBody = null!,
        string queryParams = null!, string urlPath = null!, CancellationToken ct = default)
        => throw new NotSupportedException();

    public Task<Response> MakeRequest(HttpRequestMessage request, CancellationToken ct = default)
        => throw new NotSupportedException();
}
