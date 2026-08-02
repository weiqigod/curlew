using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;

namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Uploads compiled HTML to the SendGrid Dynamic Templates API and returns the template ID.
/// Used by the <c>dev upload-templates</c> CLI subcommand, which is wired into the
/// CI release pipeline by M14-015.
/// Uses the SendGrid v3 REST API directly (the 9.x SDK does not expose template management endpoints).
/// </summary>
public sealed class SendGridTemplateUploader(HttpClient http, string apiKey)
{
    /// <summary>
    /// Creates or updates a SendGrid Dynamic Template named after the slug and activates
    /// a new version with the provided HTML. Returns the template ID.
    /// </summary>
    /// <param name="slug">Template slug used as the SendGrid template name.</param>
    /// <param name="subject">Email subject for the template version.</param>
    /// <param name="html">Compiled HTML content.</param>
    /// <param name="ct">Cancellation token.</param>
    /// <returns>The SendGrid Dynamic Template ID (e.g. <c>d-abc123</c>).</returns>
    public async Task<string> UploadAsync(string slug, string subject, string html, CancellationToken ct)
    {
        var templateId = await ResolveOrCreateTemplateAsync(slug, ct).ConfigureAwait(false);
        await CreateActiveVersionAsync(templateId, subject, html, ct).ConfigureAwait(false);
        return templateId;
    }

    private async Task<string> ResolveOrCreateTemplateAsync(string slug, CancellationToken ct)
    {
        // List existing dynamic templates to find a name match.
        using var listReq = BuildRequest(HttpMethod.Get, "/v3/templates?generations=dynamic");
        using var listResp = await http.SendAsync(listReq, ct).ConfigureAwait(false);
        listResp.EnsureSuccessStatusCode();

        var listBody = await listResp.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
        using var doc = JsonDocument.Parse(listBody);
        if (doc.RootElement.TryGetProperty("templates", out var templates))
        {
            foreach (var t in templates.EnumerateArray())
            {
                if (t.TryGetProperty("name", out var nameEl) &&
                    nameEl.GetString() == slug &&
                    t.TryGetProperty("id", out var idEl))
                {
                    return idEl.GetString()
                        ?? throw new InvalidOperationException($"SendGrid template '{slug}' has a null id.");
                }
            }
        }

        // Template not found — create a new one.
        var payload = JsonSerializer.Serialize(new { name = slug, generation = "dynamic" });
        using var createReq = BuildRequest(HttpMethod.Post, "/v3/templates");
        createReq.Content = new StringContent(payload, Encoding.UTF8, "application/json");
        using var createResp = await http.SendAsync(createReq, ct).ConfigureAwait(false);
        createResp.EnsureSuccessStatusCode();

        var createBody = await createResp.Content.ReadAsStringAsync(ct).ConfigureAwait(false);
        using var createDoc = JsonDocument.Parse(createBody);
        return createDoc.RootElement.GetProperty("id").GetString()
            ?? throw new InvalidOperationException("SendGrid did not return a template id.");
    }

    private async Task CreateActiveVersionAsync(string templateId, string subject, string html, CancellationToken ct)
    {
        var payload = JsonSerializer.Serialize(new
        {
            template_id = templateId,
            active = 1,
            name = $"v-{DateTimeOffset.UtcNow:yyyyMMddHHmmss}",
            subject,
            html_content = html,
        });

        using var req = BuildRequest(HttpMethod.Post, $"/v3/templates/{templateId}/versions");
        req.Content = new StringContent(payload, Encoding.UTF8, "application/json");
        using var resp = await http.SendAsync(req, ct).ConfigureAwait(false);
        resp.EnsureSuccessStatusCode();
    }

    private HttpRequestMessage BuildRequest(HttpMethod method, string path)
    {
        var req = new HttpRequestMessage(method, path);
        req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", apiKey);
        return req;
    }
}
