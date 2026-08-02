namespace ApiTool.Backend.Notifications.Email;

/// <summary>
/// Thread-safe bounded ring-buffer implementation of <see cref="IRecentlySentEmailLog"/>.
/// Oldest entries are evicted when capacity is reached.
/// </summary>
public sealed class InMemoryRecentlySentEmailLog : IRecentlySentEmailLog
{
    private readonly int _capacity;
    private readonly Queue<RecentlySentEmail> _entries;
    private readonly object _lock = new();

    /// <summary>Initialises the log with the specified ring-buffer capacity (must be > 0).</summary>
    public InMemoryRecentlySentEmailLog(int capacity = 200)
    {
        if (capacity <= 0) throw new ArgumentOutOfRangeException(nameof(capacity), "Capacity must be > 0.");
        _capacity = capacity;
        _entries = new Queue<RecentlySentEmail>(capacity);
    }

    /// <inheritdoc/>
    public void Record(EmailMessage message, DateTimeOffset sentAt)
    {
        var vars = (IReadOnlyDictionary<string, object>)message.Variables
            .ToDictionary(kv => kv.Key, kv => (object)kv.Value);

        var entry = new RecentlySentEmail(message.To, message.TemplateSlug, sentAt, vars);

        lock (_lock)
        {
            if (_entries.Count >= _capacity)
                _entries.Dequeue();
            _entries.Enqueue(entry);
        }
    }

    /// <inheritdoc/>
    public IReadOnlyList<RecentlySentEmail> Recent(int limit)
    {
        if (limit <= 0) return Array.Empty<RecentlySentEmail>();

        lock (_lock)
        {
            return _entries.TakeLast(limit).ToList();
        }
    }
}
