using System.Threading.Channels;
using ApiTool.Backend.Notifications;
using ApiTool.Backend.Notifications.Email;
using FluentAssertions;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Notifications.Email;

public sealed class EmailQueueProcessorTests
{
    /// <summary>
    /// Builds a processor wired to a scripted sender and recording dead-letter store.
    /// Zero-delay retries for fast tests.
    /// </summary>
    private static (
        EmailQueueProcessor Processor,
        Channel<EmailMessage> Channel,
        RecordingSmtpSender Sender,
        RecordingEmailDeadLetterStore DeadLetters)
        MakeProcessor()
    {
        var channel = System.Threading.Channels.Channel.CreateUnbounded<EmailMessage>();
        var sender = new RecordingSmtpSender();
        var deadLetters = new RecordingEmailDeadLetterStore();

        var services = new ServiceCollection();
        services.AddSingleton<ISmtpSender>(sender);
        var sp = services.BuildServiceProvider();

        var opts = Options.Create(new EmailQueueProcessorOptions
        {
            MaxAttempts = 3,
            RetryDelays = [TimeSpan.Zero, TimeSpan.Zero, TimeSpan.Zero],
        });

        var processor = new EmailQueueProcessor(
            channel, sp, opts, deadLetters, NullLogger<EmailQueueProcessor>.Instance);

        return (processor, channel, sender, deadLetters);
    }

    private static EmailMessage MakeMsg(string slug = "email_verification")
        => new("to@test.com", slug, new Dictionary<string, string> { ["first_name"] = "Alex" }, DateTimeOffset.UtcNow);

    private static async Task<bool> DrainAsync(
        EmailQueueProcessor processor,
        Channel<EmailMessage> channel,
        TimeSpan timeout)
    {
        using var cts = new CancellationTokenSource(timeout);
        channel.Writer.TryComplete();
        try
        {
            await processor.StartAsync(cts.Token);
            await processor.ExecuteTask!.WaitAsync(timeout);
            await processor.StopAsync(cts.Token);
            return true;
        }
        catch (OperationCanceledException)
        {
            return false;
        }
    }

    [Fact]
    public async Task Happy_path_invokes_SendTemplateAsync()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        var send = sender.TemplateSends.Should().ContainSingle().Which;
        send.To.Should().Be("to@test.com");
        send.Slug.Should().Be("email_verification");
        send.Variables.Should().ContainKey("first_name").WhoseValue.Should().Be("Alex");
        deadLetters.Entries.Should().BeEmpty();
    }

    [Fact]
    public async Task Three_consecutive_5xx_dead_letters_with_last_error()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        // Script 3 transient failures.
        sender.SendResults.Enqueue(new SendGridSmtpSender.SendGridUnavailableException(503));
        sender.SendResults.Enqueue(new SendGridSmtpSender.SendGridUnavailableException(503));
        sender.SendResults.Enqueue(new SendGridSmtpSender.SendGridUnavailableException(503));

        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        sender.TemplateSends.Should().HaveCount(3, because: "3 attempts before dead-lettering");
        deadLetters.Entries.Should().ContainSingle();
        deadLetters.Entries[0].AttemptCount.Should().Be(3);
        deadLetters.Entries[0].LastError.Should().Contain("503");
    }

    [Fact]
    public async Task TemplateNotFound_skips_retries_and_dead_letters_immediately()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        sender.SendResults.Enqueue(new EmailTemplateNotFoundException("email_verification"));

        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        sender.TemplateSends.Should().ContainSingle(because: "no retry on permanent failures");
        deadLetters.Entries.Should().ContainSingle();
        deadLetters.Entries[0].AttemptCount.Should().Be(1);
    }

    [Fact]
    public async Task TemplateVariableUnknown_skips_retries_and_dead_letters_immediately()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        sender.SendResults.Enqueue(
            new EmailTemplateVariableUnknownException("email_verification", ["unknown_key"]));

        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        sender.TemplateSends.Should().ContainSingle(because: "no retry on permanent failures");
        deadLetters.Entries.Should().ContainSingle();
    }

    [Fact]
    public async Task Transient_then_success_does_not_dead_letter()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        // First attempt fails; second succeeds.
        sender.SendResults.Enqueue(new SendGridSmtpSender.SendGridRateLimitedException());
        sender.SendResults.Enqueue(null); // null = success

        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        sender.TemplateSends.Should().HaveCount(2, because: "one retry before success");
        deadLetters.Entries.Should().BeEmpty();
    }

    [Fact]
    public async Task Permanent_4xx_dead_letters_immediately_without_retry()
    {
        var (processor, channel, sender, deadLetters) = MakeProcessor();
        sender.SendResults.Enqueue(new SendGridSmtpSender.SendGridPermanentException(400));

        channel.Writer.TryWrite(MakeMsg());
        await DrainAsync(processor, channel, TimeSpan.FromSeconds(5));

        sender.TemplateSends.Should().ContainSingle(because: "no retry on permanent 4xx");
        deadLetters.Entries.Should().ContainSingle(because: "immediately dead-lettered");
        deadLetters.Entries[0].AttemptCount.Should().Be(1);
    }

    [Fact]
    public async Task Cancellation_stops_processor_cleanly()
    {
        var (processor, channel, _, _) = MakeProcessor();
        // Don't complete the channel — just cancel.
        using var cts = new CancellationTokenSource();
        await processor.StartAsync(cts.Token);
        cts.Cancel();

        var completed = await Task.WhenAny(
            processor.ExecuteTask!,
            Task.Delay(TimeSpan.FromSeconds(3)));

        completed.Should().Be(processor.ExecuteTask, because: "cancellation should stop the processor");
    }

    [Fact]
    public async Task Cancellation_during_send_does_not_dead_letter()
    {
        // Arrange: a sender that blocks until the processor's stoppingToken is cancelled,
        // simulating an in-flight HTTP call interrupted by server shutdown.
        var channel = System.Threading.Channels.Channel.CreateUnbounded<EmailMessage>();
        var deadLetters = new RecordingEmailDeadLetterStore();
        var cancelSender = new CancelOnSendSmtpSender();

        var services = new ServiceCollection();
        services.AddSingleton<ISmtpSender>(cancelSender);
        var sp = services.BuildServiceProvider();

        var opts = Options.Create(new EmailQueueProcessorOptions
        {
            MaxAttempts = 3,
            RetryDelays = [TimeSpan.Zero, TimeSpan.Zero, TimeSpan.Zero],
        });

        var processor = new EmailQueueProcessor(
            channel, sp, opts, deadLetters, NullLogger<EmailQueueProcessor>.Instance);

        // Act: write a message and start the processor.
        channel.Writer.TryWrite(MakeMsg());
        await processor.StartAsync(CancellationToken.None);

        // Wait until the sender has entered SendTemplateAsync (message is in-flight).
        await cancelSender.SendStarted.Task.WaitAsync(TimeSpan.FromSeconds(5));

        // StopAsync cancels the stoppingToken — this is the real server shutdown path.
        // The sender observes ct.IsCancellationRequested == true and throws OCE.
        await processor.StopAsync(CancellationToken.None);

        // Assert: no dead-letter entry — cancellation is a clean shutdown, not a failure.
        deadLetters.Entries.Should().BeEmpty(because: "OperationCanceledException during shutdown must not produce a dead-letter");
    }

    /// <summary>
    /// Sender that blocks on <see cref="SendTemplateAsync"/> until the processor's own
    /// <paramref name="ct"/> is cancelled (mimics an in-flight HTTP call interrupted by shutdown).
    /// </summary>
    private sealed class CancelOnSendSmtpSender : ISmtpSender
    {
        public TaskCompletionSource SendStarted { get; } = new();

        public Task SendAsync(string toAddress, string subject, string body, CancellationToken ct)
            => Task.CompletedTask;

        public async Task SendTemplateAsync(
            string toAddress, string slug, IDictionary<string, string> variables, CancellationToken ct)
        {
            SendStarted.TrySetResult();
            // Block until the processor's own stoppingToken is cancelled.
            // This throws OperationCanceledException with ct as the source,
            // so ct.IsCancellationRequested == true in the catch clause.
            await Task.Delay(Timeout.Infinite, ct).ConfigureAwait(false);
        }
    }
}
