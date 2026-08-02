using ApiTool.Backend.Notifications.Email;
using FluentAssertions;

namespace ApiTool.Backend.Tests.Notifications.Email;

/// <summary>
/// Tests for <see cref="InMemoryRecentlySentEmailLog"/> ring-buffer behaviour and concurrency safety.
/// </summary>
public sealed class RecentlySentEmailLogTests
{
    private static EmailMessage MakeMsg(string slug = "billing_receipt")
        => new("to@test.com", slug, new Dictionary<string, string> { ["amount"] = "9.99" }, DateTimeOffset.UtcNow);

    [Fact]
    public void Record_under_capacity_returns_in_insertion_order()
    {
        var log = new InMemoryRecentlySentEmailLog(capacity: 10);
        var t0 = DateTimeOffset.UtcNow;
        log.Record(MakeMsg("email_verification"), t0);
        log.Record(MakeMsg("billing_receipt"), t0.AddSeconds(1));

        var recent = log.Recent(10);
        recent.Should().HaveCount(2);
        recent[0].TemplateSlug.Should().Be("email_verification");
        recent[1].TemplateSlug.Should().Be("billing_receipt");
    }

    [Fact]
    public void Record_over_capacity_evicts_oldest()
    {
        var log = new InMemoryRecentlySentEmailLog(capacity: 3);
        var t0 = DateTimeOffset.UtcNow;
        log.Record(MakeMsg("a"), t0);
        log.Record(MakeMsg("b"), t0.AddSeconds(1));
        log.Record(MakeMsg("c"), t0.AddSeconds(2));
        log.Record(MakeMsg("d"), t0.AddSeconds(3)); // evicts "a"

        var recent = log.Recent(10);
        recent.Should().HaveCount(3);
        recent[0].TemplateSlug.Should().Be("b");
        recent[1].TemplateSlug.Should().Be("c");
        recent[2].TemplateSlug.Should().Be("d");
    }

    [Fact]
    public void Recent_with_limit_zero_returns_empty()
    {
        var log = new InMemoryRecentlySentEmailLog(capacity: 10);
        log.Record(MakeMsg(), DateTimeOffset.UtcNow);

        var recent = log.Recent(0);
        recent.Should().BeEmpty();
    }

    [Fact]
    public async Task Concurrent_record_does_not_lose_messages()
    {
        const int n = 1000;
        var log = new InMemoryRecentlySentEmailLog(capacity: n);
        var t0 = DateTimeOffset.UtcNow;

        var tasks = Enumerable.Range(0, n)
            .Select(i => Task.Run(() => log.Record(MakeMsg($"slug_{i}"), t0.AddMilliseconds(i))))
            .ToArray();

        await Task.WhenAll(tasks);

        // All n messages should be present (capacity == n, none evicted).
        log.Recent(n).Should().HaveCount(n);
    }
}
