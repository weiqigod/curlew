using System.Net;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// xUnit class fixture that resolves the stripe-mock base URL and skips the test class
/// when stripe-mock is not reachable. Configured via APITOOL__STRIPE__APIBASE env var.
/// </summary>
public sealed class StripeMockFixture : IAsyncLifetime
{
    /// <summary>Base URL of the stripe-mock server (e.g. http://localhost:12111).</summary>
    public string BaseUrl { get; } =
        Environment.GetEnvironmentVariable("APITOOL__STRIPE__APIBASE")
        ?? "http://localhost:12111";

    /// <summary><see langword="true"/> when stripe-mock responded to a probe request.</summary>
    public bool IsAvailable { get; private set; }

    /// <inheritdoc/>
    public async Task InitializeAsync()
    {
        try
        {
            using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(3) };
            // stripe-mock returns 401 on /v1/customers without auth — that signals it is live.
            var resp = await http.GetAsync($"{BaseUrl}/v1/customers");
            IsAvailable = resp.StatusCode is HttpStatusCode.Unauthorized or HttpStatusCode.OK;
        }
        catch
        {
            IsAvailable = false;
        }

        if (!IsAvailable)
            throw new Xunit.SkipException(
                $"stripe-mock not reachable at {BaseUrl}; skipping stripe-integration tests. " +
                "Start it with: docker compose -f docker-compose.test.yml up -d stripe-mock");
    }

    /// <inheritdoc/>
    public Task DisposeAsync() => Task.CompletedTask;
}
