using ApiTool.Backend.Storage;

namespace ApiTool.Backend.Tests.Storage;

/// <summary>Unit tests for <see cref="InMemoryObjectStore"/>.</summary>
public class InMemoryObjectStoreTests
{
    [Fact]
    public async Task Put_then_Get_returns_same_bytes()
    {
        var store = new InMemoryObjectStore();
        var payload = """{"hello":"world"}"""u8.ToArray();
        await store.PutAsync("exports/u/r.json", new MemoryStream(payload), "application/json", default);

        await using var read = await store.GetAsync("exports/u/r.json", default);
        using var reader = new StreamReader(read);
        var text = await reader.ReadToEndAsync();
        text.Should().Be("""{"hello":"world"}""");
    }

    [Fact]
    public async Task Get_nonexistent_key_throws_KeyNotFoundException()
    {
        var store = new InMemoryObjectStore();
        await FluentActions.Awaiting(() => store.GetAsync("does-not-exist", default))
            .Should().ThrowAsync<KeyNotFoundException>();
    }

    [Fact]
    public async Task SignedUrl_carries_expiry_query_param()
    {
        var store = new InMemoryObjectStore();
        await store.PutAsync("k", new MemoryStream(Array.Empty<byte>()), "application/json", default);
        var url = await store.GetSignedUrlAsync("k", TimeSpan.FromHours(24), default);
        url.Query.Should().Contain("exp=");
    }

    [Fact]
    public async Task SignedUrl_is_retrievable_and_correct_format()
    {
        var store = new InMemoryObjectStore();
        await store.PutAsync("exports/test.json", new MemoryStream("""{"ok":1}"""u8.ToArray()), "application/json", default);
        var url = await store.GetSignedUrlAsync("exports/test.json", TimeSpan.FromHours(24), default);
        url.Should().NotBeNull();
        url.AbsoluteUri.Should().StartWith("http://");
        url.Query.Should().Contain("exp=");
    }

    [Fact]
    public async Task Put_overwrites_existing_key()
    {
        var store = new InMemoryObjectStore();
        await store.PutAsync("k", new MemoryStream("v1"u8.ToArray()), "text/plain", default);
        await store.PutAsync("k", new MemoryStream("v2"u8.ToArray()), "text/plain", default);

        await using var read = await store.GetAsync("k", default);
        using var reader = new StreamReader(read);
        var text = await reader.ReadToEndAsync();
        text.Should().Be("v2");
    }
}
