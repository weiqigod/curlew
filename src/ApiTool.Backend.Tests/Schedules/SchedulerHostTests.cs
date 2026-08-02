using ApiTool.Backend.Schedules;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;

namespace ApiTool.Backend.Tests.Schedules;

/// <summary>Tests for <see cref="SchedulerHost"/> background service behavior.</summary>
public sealed class SchedulerHostTests
{
    [Fact]
    public async Task ExecuteAsync_calls_enqueue_due_on_tick()
    {
        var enqueueCallCount = 0;
        var fakeService = new FakeSchedulesService(() =>
        {
            Interlocked.Increment(ref enqueueCallCount);
            return Task.FromResult(0);
        });

        var scopeFactory = new FakeScopeFactory(fakeService);
        var logger = NullLogger<SchedulerHost>.Instance;
        var tickInterval = TimeSpan.FromMilliseconds(50);

        var host = new SchedulerHost(scopeFactory, logger, tickInterval);
        using var cts = new CancellationTokenSource(TimeSpan.FromMilliseconds(300));

        await host.StartAsync(cts.Token);

        // Wait for the host to tick at least once
        await Task.Delay(200);
        await host.StopAsync(default);

        enqueueCallCount.Should().BeGreaterThan(0, because: "the scheduler should have called EnqueueDueAsync at least once");
    }

    [Fact]
    public async Task ExecuteAsync_logs_info_when_schedules_are_due()
    {
        var logger = new CountingLogger<SchedulerHost>();
        var fakeService = new FakeSchedulesService(() => Task.FromResult(3));  // 3 due schedules
        var tickInterval = TimeSpan.FromMilliseconds(50);

        var scopeFactory = new FakeScopeFactory(fakeService);
        var host = new SchedulerHost(scopeFactory, logger, tickInterval);

        using var cts = new CancellationTokenSource(TimeSpan.FromMilliseconds(300));
        await host.StartAsync(cts.Token);
        await Task.Delay(200);
        await host.StopAsync(default);

        logger.InfoMessages.Should().Contain(
            msg => msg.Contains("scheduler tick"),
            because: "the scheduler should emit 'scheduler tick {Count} schedules due' when schedules are enqueued");
    }

    [Fact]
    public async Task ExecuteAsync_survives_transient_exceptions()
    {
        var callCount = 0;
        var fakeService = new FakeSchedulesService(() =>
        {
            var n = Interlocked.Increment(ref callCount);
            if (n == 1)
                throw new InvalidOperationException("transient failure");
            return Task.FromResult(0);
        });

        var scopeFactory = new FakeScopeFactory(fakeService);
        var logger = NullLogger<SchedulerHost>.Instance;
        var tickInterval = TimeSpan.FromMilliseconds(50);
        var host = new SchedulerHost(scopeFactory, logger, tickInterval);

        using var cts = new CancellationTokenSource(TimeSpan.FromMilliseconds(300));
        await host.StartAsync(cts.Token);
        await Task.Delay(400);
        await host.StopAsync(default);

        callCount.Should().BeGreaterThan(1,
            because: "host should continue ticking after a transient exception");
    }

    // ── Fakes ─────────────────────────────────────────────────────────────────

    private sealed class FakeSchedulesService(Func<Task<int>> enqueueCallback) : ISchedulerEnqueuer
    {
        public Task<int> EnqueueDueAsync(CancellationToken ct) => enqueueCallback();
    }

    private sealed class FakeScopeFactory(ISchedulerEnqueuer enqueuer) : IServiceScopeFactory
    {
        public IServiceScope CreateScope()
        {
            var services = new ServiceCollection();
            services.AddSingleton(enqueuer);
            var provider = services.BuildServiceProvider();
            return provider.CreateScope();
        }
    }

    private sealed class CountingLogger<T> : ILogger<T>
    {
        private int _infoCount;
        private readonly System.Collections.Concurrent.ConcurrentBag<string> _infoMessages = new();

        public int InfoCount => _infoCount;
        public IEnumerable<string> InfoMessages => _infoMessages;

        public IDisposable? BeginScope<TState>(TState state) where TState : notnull => null;

        public bool IsEnabled(LogLevel logLevel) => true;

        public void Log<TState>(LogLevel logLevel, EventId eventId, TState state, Exception? exception, Func<TState, Exception?, string> formatter)
        {
            if (logLevel == LogLevel.Information)
            {
                Interlocked.Increment(ref _infoCount);
                _infoMessages.Add(formatter(state, exception));
            }
        }
    }
}
