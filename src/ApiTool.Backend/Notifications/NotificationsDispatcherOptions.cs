namespace ApiTool.Backend.Notifications;

/// <summary>Options that control retry behaviour of <see cref="NotificationsDispatcher"/>.</summary>
public sealed class NotificationsDispatcherOptions
{
    /// <summary>
    /// Delays to wait between retry attempts. The number of retries equals the length of this array.
    /// Defaults to two retries at 500 ms and 2 s.
    /// </summary>
    public TimeSpan[] RetryDelays { get; set; } =
    [
        TimeSpan.FromMilliseconds(500),
        TimeSpan.FromSeconds(2),
    ];
}
