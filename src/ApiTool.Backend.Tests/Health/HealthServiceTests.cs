using ApiTool.Backend.Health;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Health;

public sealed class HealthServiceTests
{
    private sealed class FakeDb(bool ok, bool throws = false) : IDbHealthProbe
    {
        public Task<bool> IsConnectedAsync(CancellationToken ct) =>
            throws ? Task.FromException<bool>(new InvalidOperationException("db")) : Task.FromResult(ok);
    }

    private sealed class FakeRedis(bool ok, bool configured = true, bool throws = false) : IRedisHealthProbe
    {
        public bool IsConfigured => configured;

        public Task<bool> IsConnectedAsync(CancellationToken ct) =>
            throws ? Task.FromException<bool>(new InvalidOperationException("redis")) : Task.FromResult(ok);
    }

    [Theory]
    [InlineData(true,  true,  true,  "healthy",   "connected",    "connected")]
    [InlineData(false, true,  true,  "unhealthy", "disconnected", "connected")]
    [InlineData(true,  false, true,  "unhealthy", "connected",    "disconnected")]
    [InlineData(true,  true,  false, "healthy",   "connected",    "not_configured")]
    [InlineData(false, false, false, "unhealthy", "disconnected", "not_configured")]
    public async Task Check_returns_expected_report(
        bool dbOk, bool redisOk, bool redisConfigured,
        string wantStatus, string wantDb, string wantRedis)
    {
        var svc = new HealthService(
            new FakeDb(dbOk),
            new FakeRedis(redisOk, redisConfigured),
            NullLogger<HealthService>.Instance);
        var rep = await svc.CheckAsync(CancellationToken.None);
        rep.Status.Should().Be(wantStatus);
        rep.Db.Should().Be(wantDb);
        rep.Redis.Should().Be(wantRedis);
    }

    [Fact]
    public async Task Check_treats_probe_exception_as_disconnected()
    {
        var svc = new HealthService(
            new FakeDb(true, throws: true),
            new FakeRedis(true),
            NullLogger<HealthService>.Instance);
        var rep = await svc.CheckAsync(CancellationToken.None);
        rep.Status.Should().Be("unhealthy");
        rep.Db.Should().Be("disconnected");
    }
}
