using System.Net;
using ApiTool.Backend.Health;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;

namespace ApiTool.Backend.Tests.Health;

[Collection(BackendCollection.Name)]
public sealed class HealthEndpointTests(BackendFactory factory) : IAsyncLifetime
{
    public Task InitializeAsync() => factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    private HttpClient NewClientWith(bool dbOk, bool redisOk, bool redisConfigured)
    {
        return factory.WithWebHostBuilder(b =>
        {
            b.ConfigureServices(s =>
            {
                s.RemoveAll<IDbHealthProbe>();
                s.AddSingleton<IDbHealthProbe>(new FakeDb(dbOk));
                s.RemoveAll<IRedisHealthProbe>();
                s.AddSingleton<IRedisHealthProbe>(new FakeRedis(redisOk, redisConfigured));
            });
        }).CreateClient();
    }

    [Fact]
    public async Task Health_returns_200_when_both_up()
    {
        var client = NewClientWith(true, true, true);
        var resp = await client.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"status\":\"healthy\"");
        body.Should().Contain("\"db\":\"connected\"");
        body.Should().Contain("\"redis\":\"connected\"");
    }

    [Fact]
    public async Task Health_returns_503_when_db_down()
    {
        var client = NewClientWith(false, true, true);
        var resp = await client.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"status\":\"unhealthy\"");
        body.Should().Contain("\"db\":\"disconnected\"");
        body.Should().Contain("\"redis\":\"connected\"");
    }

    [Fact]
    public async Task Health_returns_503_when_redis_down()
    {
        var client = NewClientWith(true, false, true);
        var resp = await client.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.ServiceUnavailable);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"status\":\"unhealthy\"");
        body.Should().Contain("\"db\":\"connected\"");
        body.Should().Contain("\"redis\":\"disconnected\"");
    }

    [Fact]
    public async Task Health_returns_200_when_redis_not_configured()
    {
        var client = NewClientWith(true, false, false);
        var resp = await client.GetAsync("/health");
        resp.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await resp.Content.ReadAsStringAsync();
        body.Should().Contain("\"redis\":\"not_configured\"");
    }

    [Fact]
    public async Task Health_does_not_require_authentication()
    {
        var client = NewClientWith(true, true, true);
        // No Authorization header set — anonymous access must be allowed.
        var resp = await client.GetAsync("/health");
        resp.StatusCode.Should().NotBe(HttpStatusCode.Unauthorized);
    }

    private sealed class FakeDb(bool ok) : IDbHealthProbe
    {
        public Task<bool> IsConnectedAsync(CancellationToken ct) => Task.FromResult(ok);
    }

    private sealed class FakeRedis(bool ok, bool configured) : IRedisHealthProbe
    {
        public bool IsConfigured => configured;
        public Task<bool> IsConnectedAsync(CancellationToken ct) => Task.FromResult(ok);
    }
}
