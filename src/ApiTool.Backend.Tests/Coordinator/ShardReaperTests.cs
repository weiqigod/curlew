using ApiTool.Backend.Coordinator;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Coordinator;

/// <summary>Tests for <see cref="ShardReaper"/> background service behavior.</summary>
public sealed class ShardReaperTests
{
    [Fact]
    public async Task ExecuteAsync_calls_reap_on_tick()
    {
        var callCount = 0;
        var fakeReaper = new FakeShardReaper(() =>
        {
            Interlocked.Increment(ref callCount);
            return Task.FromResult(0);
        });

        var scopeFactory = new FakeScopeFactory(fakeReaper);
        var logger = NullLogger<ShardReaper>.Instance;
        var tickInterval = TimeSpan.FromMilliseconds(50);
        var host = new ShardReaper(scopeFactory, logger, tickInterval);

        // StartAsync takes a startup cancellation token (not the run-loop token).
        // The run-loop is stopped exclusively via StopAsync.
        await host.StartAsync(CancellationToken.None);
        await Task.Delay(200);
        await host.StopAsync(default);

        callCount.Should().BeGreaterThan(0,
            because: "the reaper should have called ReapStaleShardsAsync at least once");
    }

    [Fact]
    public async Task ExecuteAsync_logs_info_when_shards_are_reclaimed()
    {
        var logger = new CountingLogger<ShardReaper>();
        var fakeReaper = new FakeShardReaper(() => Task.FromResult(3)); // 3 shards reclaimed

        var scopeFactory = new FakeScopeFactory(fakeReaper);
        var tickInterval = TimeSpan.FromMilliseconds(50);
        var host = new ShardReaper(scopeFactory, logger, tickInterval);

        // StartAsync takes a startup cancellation token (not the run-loop token).
        // The run-loop is stopped exclusively via StopAsync.
        await host.StartAsync(CancellationToken.None);
        await Task.Delay(200);
        await host.StopAsync(default);

        logger.InfoMessages.Should().Contain(
            msg => msg.Contains("reclaimed"),
            because: "the reaper should log when shards are reclaimed");
    }

    [Fact]
    public async Task ExecuteAsync_survives_transient_exceptions()
    {
        var callCount = 0;
        var fakeReaper = new FakeShardReaper(() =>
        {
            var n = Interlocked.Increment(ref callCount);
            if (n == 1)
                throw new InvalidOperationException("transient failure");
            return Task.FromResult(0);
        });

        var scopeFactory = new FakeScopeFactory(fakeReaper);
        var logger = NullLogger<ShardReaper>.Instance;
        var tickInterval = TimeSpan.FromMilliseconds(50);
        var host = new ShardReaper(scopeFactory, logger, tickInterval);

        // StartAsync takes a startup cancellation token (not the run-loop token).
        // The run-loop is stopped exclusively via StopAsync.
        await host.StartAsync(CancellationToken.None);
        await Task.Delay(400);
        await host.StopAsync(default);

        callCount.Should().BeGreaterThan(1,
            because: "host should continue ticking after a transient exception");
    }

    // ── Fakes ──────────────────────────────────────────────────────────────────

    private sealed class FakeShardReaper(Func<Task<int>> reapCallback) : IShardReaper
    {
        public Task<int> ReapStaleShardsAsync(CancellationToken ct) => reapCallback();
    }

    private sealed class FakeScopeFactory(IShardReaper reaper) : IServiceScopeFactory
    {
        public IServiceScope CreateScope()
        {
            var services = new ServiceCollection();
            services.AddSingleton(reaper);
            var provider = services.BuildServiceProvider();
            return provider.CreateScope();
        }
    }

    private sealed class CountingLogger<T> : Microsoft.Extensions.Logging.ILogger<T>
    {
        private readonly System.Collections.Concurrent.ConcurrentBag<string> _infoMessages = new();

        public IEnumerable<string> InfoMessages => _infoMessages;

        public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;
        public bool IsEnabled(Microsoft.Extensions.Logging.LogLevel logLevel) => true;

        public void Log<TState>(
            Microsoft.Extensions.Logging.LogLevel logLevel,
            Microsoft.Extensions.Logging.EventId eventId,
            TState state,
            Exception? exception,
            Func<TState, Exception?, string> formatter)
        {
            if (logLevel == Microsoft.Extensions.Logging.LogLevel.Information)
                _infoMessages.Add(formatter(state, exception));
        }
    }
}
