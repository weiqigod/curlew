// Refs: M16-016 — production implementation of IGitLabProjectLookup.
using System.Net;
using System.Net.Security;
using System.Security.Cryptography.X509Certificates;
using System.Text.Json;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.GitLab.Installations;

/// <summary>
/// Production implementation of <see cref="IGitLabProjectLookup"/>.
/// Issues a GET request to <c>{baseUrl}/api/v4/projects/{encoded-path}</c> using the
/// named <c>gitlab-projects</c> HTTP client from <see cref="IHttpClientFactory"/>.
/// When a PEM CA bundle is provided the request uses a request-scoped <see cref="HttpClient"/>
/// with a custom <see cref="HttpClientHandler"/> that trusts the supplied certificate, enabling
/// self-managed GitLab instances with private CAs to be validated during integration creation.
/// </summary>
/// <param name="httpClientFactory">Factory for the named <c>gitlab-projects</c> client (standard path).</param>
/// <param name="options">GitLab feature options.</param>
/// <param name="caBundleHandlerFactory">
/// Optional factory invoked with the PEM string to produce an <see cref="HttpMessageHandler"/>
/// for the CA-bundle path. When <see langword="null"/> (production default), a real
/// <see cref="HttpClientHandler"/> with <see cref="HttpClientHandler.ServerCertificateCustomValidationCallback"/>
/// is used. Inject a custom factory in tests to intercept or simulate CA-bundle requests.
/// </param>
public sealed class GitLabProjectLookup(
    IHttpClientFactory httpClientFactory,
    IOptions<GitLabOptions> options,
    Func<string, HttpMessageHandler>? caBundleHandlerFactory = null) : IGitLabProjectLookup
{
    /// <inheritdoc />
    public async Task<GitLabProjectLookupResult> LookupAsync(
        string gitlabBaseUrl,
        string projectPath,
        string accessToken,
        string? caBundlePem,
        CancellationToken ct)
    {
        // Reject insecure base URLs unless AllowHttp is explicitly enabled.
        if (!options.Value.Poster.AllowHttp &&
            gitlabBaseUrl.StartsWith("http://", StringComparison.OrdinalIgnoreCase))
        {
            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.InsecureBaseUrl, null, null);
        }

        var encodedPath = EncodeProjectPath(projectPath);
        var normalizedBase = gitlabBaseUrl.TrimEnd('/');

        // Guard against syntactically invalid base URLs (e.g. "not-a-url", "ftp://example.com/with path").
        // The Uri ctor with UriKind.Absolute would throw UriFormatException for these inputs, which
        // propagates as an unhandled 500 to the caller. Return a typed error instead.
        if (!Uri.TryCreate($"{normalizedBase}/api/v4/projects/{encodedPath}", UriKind.Absolute, out var requestUri))
        {
            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidBaseUrl, null, null);
        }

        // When a CA bundle is provided, use a request-scoped client with a custom handler
        // so the private CA is trusted only for this one-shot lookup.
        // The caCert and customClient are disposed AFTER the response body has been read
        // so that the body stream is fully consumed before any underlying resources are torn down.
        if (!string.IsNullOrWhiteSpace(caBundlePem))
        {
            return await LookupWithCaBundleAsync(requestUri, accessToken, caBundlePem, caBundleHandlerFactory, ct);
        }

        var client = httpClientFactory.CreateClient("gitlab-projects");

        using var request = new HttpRequestMessage(HttpMethod.Get, requestUri);
        request.Headers.TryAddWithoutValidation("Private-Token", accessToken);

        HttpResponseMessage response;
        try
        {
            response = await client.SendAsync(request, HttpCompletionOption.ResponseContentRead, ct);
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException or OperationCanceledException
                                   && !ct.IsCancellationRequested)
        {
            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.Unreachable, null, null);
        }

        using (response)
        {
            return response.StatusCode switch
            {
                HttpStatusCode.Unauthorized => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unauthorized, null, null),

                HttpStatusCode.Forbidden => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unauthorized, null, null),

                HttpStatusCode.NotFound => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.NotFound, null, null),

                HttpStatusCode.OK => await ParseOkResponseAsync(response, ct),

                var code when (int)code >= 500 => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unreachable, null, null),

                _ => new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null),
            };
        }
    }

    // ── private helpers ───────────────────────────────────────────────────────

    /// <summary>
    /// Executes the project lookup using a request-scoped <see cref="HttpClient"/> that trusts
    /// the PEM CA bundle supplied by the caller. All resources — the <see cref="X509Certificate2"/>,
    /// the <see cref="HttpClientHandler"/>, the client, and the response — are disposed only AFTER
    /// the response body has been fully read, ensuring the lazy body stream (if any) is not torn
    /// down prematurely.
    /// </summary>
    /// <param name="handlerFactory">
    /// When non-null, invoked with <paramref name="caBundlePem"/> to produce the handler to use
    /// (test seam). When null, a production <see cref="HttpClientHandler"/> with a custom
    /// <see cref="HttpClientHandler.ServerCertificateCustomValidationCallback"/> is created.
    /// </param>
    private static async Task<GitLabProjectLookupResult> LookupWithCaBundleAsync(
        Uri requestUri,
        string accessToken,
        string caBundlePem,
        Func<string, HttpMessageHandler>? handlerFactory,
        CancellationToken ct)
    {
        HttpMessageHandler customHandler;
        X509Certificate2? caCert = null;

        if (handlerFactory is not null)
        {
            // Test seam: caller supplies the handler, no real X509 work done.
            customHandler = handlerFactory(caBundlePem);
        }
        else
        {
            // Production path: build a handler that trusts the supplied CA.
            caCert = X509Certificate2.CreateFromPem(caBundlePem);
            var h = new HttpClientHandler();
            h.ServerCertificateCustomValidationCallback = (_, cert, chain, errors) =>
            {
                if (errors == SslPolicyErrors.None) return true;
                // Accept the certificate if it chains to the supplied CA.
                chain!.ChainPolicy.ExtraStore.Add(caCert);
                chain.ChainPolicy.VerificationFlags = X509VerificationFlags.AllowUnknownCertificateAuthority;
                return chain.Build(cert!);
            };
            customHandler = h;
        }

        // Dispose order: caCert → customHandler → customClient — all happen AFTER body read.
        using var _ = caCert;
        using var handlerDisposable = customHandler;
        using var customClient = new HttpClient(customHandler, disposeHandler: false)
        {
            Timeout = TimeSpan.FromSeconds(10)
        };

        using var request = new HttpRequestMessage(HttpMethod.Get, requestUri);
        request.Headers.TryAddWithoutValidation("Private-Token", accessToken);

        HttpResponseMessage response;
        try
        {
            // ResponseContentRead ensures the full body is buffered before SendAsync returns,
            // so disposing the client (and handler) after this call is safe.
            response = await customClient.SendAsync(request, HttpCompletionOption.ResponseContentRead, ct);
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException or OperationCanceledException
                                   && !ct.IsCancellationRequested)
        {
            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.Unreachable, null, null);
        }

        // The response body is fully buffered at this point; disposing the client is safe.
        using (response)
        {
            return response.StatusCode switch
            {
                HttpStatusCode.Unauthorized => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unauthorized, null, null),

                HttpStatusCode.Forbidden => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unauthorized, null, null),

                HttpStatusCode.NotFound => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.NotFound, null, null),

                HttpStatusCode.OK => await ParseOkResponseAsync(response, ct),

                var code when (int)code >= 500 => new GitLabProjectLookupResult(
                    GitLabProjectLookupStatus.Unreachable, null, null),

                _ => new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null),
            };
        }
    }

    /// <summary>
    /// URL-encodes a project path so that slashes become <c>%2F</c> per the GitLab API spec.
    /// Each path segment is individually percent-encoded, then joined with <c>%2F</c>.
    /// </summary>
    private static string EncodeProjectPath(string projectPath)
    {
        // Uri.EscapeDataString percent-encodes everything except unreserved chars.
        // We want the *whole* path (including slashes) encoded so GitLab sees one token.
        var segments = projectPath.Split('/');
        var encoded = segments.Select(s => Uri.EscapeDataString(s));
        return string.Join("%2F", encoded);
    }

    private static async Task<GitLabProjectLookupResult> ParseOkResponseAsync(
        HttpResponseMessage response, CancellationToken ct)
    {
        try
        {
            var content = await response.Content.ReadAsStringAsync(ct);
            using var doc = JsonDocument.Parse(content);

            if (!doc.RootElement.TryGetProperty("id", out var idElement))
                return new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null);

            // TryGetInt64 throws InvalidOperationException when the element kind is not Number.
            // Guard the kind explicitly so we return InvalidResponse for any non-numeric "id".
            if (idElement.ValueKind != System.Text.Json.JsonValueKind.Number
                || !idElement.TryGetInt64(out var projectId))
                return new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null);

            var resolvedPath = doc.RootElement.TryGetProperty("path_with_namespace", out var pathEl)
                ? pathEl.GetString()
                : null;

            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.Ok, projectId, resolvedPath);
        }
        catch (JsonException)
        {
            return new GitLabProjectLookupResult(GitLabProjectLookupStatus.InvalidResponse, null, null);
        }
    }
}
