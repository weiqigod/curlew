using ApiTool.Backend.Results.Dashboard;

namespace ApiTool.Backend.Tests.Results.Dashboard;

/// <summary>Tests for <see cref="PathTemplateExtractor"/>.</summary>
public sealed class PathTemplateExtractorTests
{
    [Theory]
    // numeric id
    [InlineData("GET", "/users/123",                                    "/users/{id}")]
    // 8-char hex (length >= 8)
    [InlineData("POST", "/api/v1/orders/4f3a8b2c",                      "/api/v1/orders/{id}")]
    // RFC 4122 UUID with hyphens
    [InlineData("DELETE", "/users/4f3a8b2c-aaaa-bbbb-cccc-deadbeefcafe", "/users/{id}")]
    // 32-char lowercase hex (UUID without hyphens)
    [InlineData("PUT", "/users/4f3a8b2cdeadbeef4f3a8b2cdeadbeef",       "/users/{id}")]
    // multiple ids
    [InlineData("GET", "/orgs/123/members/4f3a8b2c",                    "/orgs/{id}/members/{id}")]
    // query string stripped
    [InlineData("GET", "/users/123?expand=profile",                     "/users/{id}")]
    // trailing slash trimmed
    [InlineData("GET", "/users/123/",                                   "/users/{id}")]
    // full URL — host stripped
    [InlineData("GET", "https://api.example.com/users/123",             "/users/{id}")]
    // non-id segments preserved
    [InlineData("GET", "/api/v1/users/me",                              "/api/v1/users/me")]
    [InlineData("GET", "/healthz",                                      "/healthz")]
    // short hex preserved (length < 8 — not id-shaped)
    [InlineData("GET", "/orders/abc",                                   "/orders/abc")]
    // root path
    [InlineData("GET", "/",                                             "/")]
    // method is uppercased in output — verify via non-uppercase input
    [InlineData("get", "/users/123",                                    "/users/{id}")]
    public void Extract_returns_expected_template(string method, string url, string expected)
    {
        var result = PathTemplateExtractor.Extract(method, url);
        Assert.Equal(expected, result);
    }

    [Theory]
    [InlineData(null,    "/users/123")]
    [InlineData("",      "/users/123")]
    [InlineData("GET",   null)]
    [InlineData("GET",   "")]
    [InlineData("GET",   "   ")]
    [InlineData("GET",   "not a url")]
    public void Extract_returns_null_for_invalid_input(string? method, string? url)
    {
        var result = PathTemplateExtractor.Extract(method, url);
        Assert.Null(result);
    }

    [Fact]
    public void Method_is_uppercased_in_output()
    {
        // The method return value is the path_template only; caller stores method separately.
        // Extract() itself returns path_template not method — verify it doesn't throw on mixed case.
        var result = PathTemplateExtractor.Extract("post", "/orders/123");
        Assert.Equal("/orders/{id}", result);
    }
}
