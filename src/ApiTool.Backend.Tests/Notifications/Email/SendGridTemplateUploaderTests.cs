using System.Net;
using System.Text.Json;
using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class SendGridTemplateUploaderTests
{
    [Fact]
    public async Task UploadAsync_creates_template_and_returns_id_when_no_existing_template()
    {
        var requests = new List<(string Method, string Path)>();
        var handler = new ScriptedHttpHandler(req =>
        {
            requests.Add((req.Method.Method, req.RequestUri!.PathAndQuery));
            return req.RequestUri.PathAndQuery switch
            {
                string p when p.StartsWith("/v3/mail/send") =>
                    new HttpResponseMessage(HttpStatusCode.OK),
                "/v3/templates?generations=dynamic" =>
                    Json("""{"templates":[]}"""),
                "/v3/templates" =>
                    Json("""{"id":"d-new123"}"""),
                string p when p.StartsWith("/v3/templates/d-new123/versions") =>
                    Json("""{"id":"v-1","active":1}"""),
                _ => new HttpResponseMessage(HttpStatusCode.NotFound)
            };
        });

        var http = new HttpClient(handler) { BaseAddress = new Uri("https://api.sendgrid.com") };
        var uploader = new SendGridTemplateUploader(http, "SG.fake_key");

        var id = await uploader.UploadAsync("email_verification", "Subject", "<html/>", default);
        id.Should().Be("d-new123");
        requests.Should().Contain(r => r.Method == "POST" && r.Path == "/v3/templates");
    }

    [Fact]
    public async Task UploadAsync_reuses_existing_template_when_slug_already_uploaded()
    {
        var handler = new ScriptedHttpHandler(req =>
        {
            return req.RequestUri!.PathAndQuery switch
            {
                "/v3/templates?generations=dynamic" =>
                    Json("""{"templates":[{"id":"d-existing","name":"email_verification"}]}"""),
                string p when p.StartsWith("/v3/templates/d-existing/versions") =>
                    Json("""{"id":"v-2","active":1}"""),
                _ => new HttpResponseMessage(HttpStatusCode.NotFound)
            };
        });

        var http = new HttpClient(handler) { BaseAddress = new Uri("https://api.sendgrid.com") };
        var uploader = new SendGridTemplateUploader(http, "SG.fake_key");

        var id = await uploader.UploadAsync("email_verification", "Subject", "<html/>", default);
        id.Should().Be("d-existing");
    }

    [Fact]
    public async Task UploadAsync_throws_on_5xx_from_list_endpoint()
    {
        var handler = new ScriptedHttpHandler(_ =>
            new HttpResponseMessage(HttpStatusCode.InternalServerError));

        var http = new HttpClient(handler) { BaseAddress = new Uri("https://api.sendgrid.com") };
        var uploader = new SendGridTemplateUploader(http, "SG.fake_key");

        await Assert.ThrowsAsync<HttpRequestException>(
            () => uploader.UploadAsync("email_verification", "Subject", "<html/>", default));
    }

    private static HttpResponseMessage Json(string json)
    {
        var resp = new HttpResponseMessage(HttpStatusCode.OK);
        resp.Content = new StringContent(json, System.Text.Encoding.UTF8, "application/json");
        return resp;
    }
}

/// <summary>Scripted <see cref="HttpMessageHandler"/> for testing HTTP clients.</summary>
internal sealed class ScriptedHttpHandler(Func<HttpRequestMessage, HttpResponseMessage> handler)
    : HttpMessageHandler
{
    protected override Task<HttpResponseMessage> SendAsync(
        HttpRequestMessage request, CancellationToken ct)
        => Task.FromResult(handler(request));
}
