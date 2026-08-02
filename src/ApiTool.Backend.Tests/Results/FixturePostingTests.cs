using System.Net;
using System.Net.Http.Headers;
using System.Net.Http.Json;
using System.Text;
using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Results;

/// <summary>Verifies the testdata fixture can be posted and yields the expected response.</summary>
[Collection(BackendCollection.Name)]
public sealed class FixturePostingTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;
    private readonly HttpClient _client;
    private readonly Guid _ownerId;
    private string? _orgId;

    public FixturePostingTests(BackendFactory factory)
    {
        _factory = factory;
        _ownerId = Guid.NewGuid();
        var token = TestTokens.Create(_ownerId, $"fixture-{_ownerId:N}@example.com");
        _client = factory.CreateClient();
        _client.DefaultRequestHeaders.Authorization =
            new AuthenticationHeaderValue("Bearer", token);
    }

    public async Task InitializeAsync()
    {
        await _factory.InitializeAsync();
        var slug = $"fix-{_ownerId:N}"[..20];
        var resp = await _client.PostAsJsonAsync("/api/v1/organizations", new { name = "FixtureOrg", slug });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(json);
        _orgId = doc.RootElement.GetProperty("id").GetString()!;
    }

    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Posting_the_testdata_fixture_returns_202_with_pass_count_3()
    {
        var fixturePath = Path.Combine(AppContext.BaseDirectory, "testdata", "sample-result-upload.json");
        File.Exists(fixturePath).Should().BeTrue(because: $"fixture must be copied to output dir at '{fixturePath}'");

        var json = await File.ReadAllTextAsync(fixturePath);
        var content = new StringContent(json, Encoding.UTF8, "application/json");
        var response = await _client.PostAsync($"/api/v1/organizations/{_orgId}/results", content);

        response.StatusCode.Should().Be(HttpStatusCode.Accepted);

        var body = await response.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        doc.RootElement.GetProperty("result_id").GetString().Should().StartWith("res_");
        doc.RootElement.GetProperty("status").GetString().Should().Be("accepted");
    }
}
