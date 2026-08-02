namespace ApiTool.Backend.Notifications.Email;

/// <summary>Configuration options for <see cref="EmailQueueProcessor"/>.</summary>
public sealed class EmailQueueProcessorOptions
{
    /// <summary>The IConfiguration section key this options class is bound from.</summary>
    public const string Section = "ApiTool:Email:Processor";

    /// <summary>Maximum number of delivery attempts before dead-lettering.</summary>
    public int MaxAttempts { get; set; } = 3;

    /// <summary>
    /// Delays between retry attempts. The delay at index <c>attempt - 1</c> is used.
    /// Defaults to exponential backoff: 500ms, 2s, 5s.
    /// Tests inject <c>[TimeSpan.Zero, ...]</c> to make retries instant.
    /// </summary>
    public TimeSpan[] RetryDelays { get; set; } =
    [
        TimeSpan.FromMilliseconds(500),
        TimeSpan.FromSeconds(2),
        TimeSpan.FromSeconds(5),
    ];
}
