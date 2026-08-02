// Unit tests for InternalAccessFilter — access-control logic for /internal/keys/*.
// The filter is bypassed in Testing/Development environments; these tests simulate Production.
using System.Net;
using ApiTool.Backend.Licensing.Keys;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Hosting;
using Microsoft.Extensions.Configuration;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Licensing;

public sealed class InternalAccessFilterTests
{
    [Fact]
    public async Task Filter_returns_404_for_non_loopback_request_in_production()
    {
        // Arrange: build a "Production" context with an external IP and no secret configured.
        var ctx = BuildContext(environment: "Production", remoteIp: "203.0.113.1", secret: null, headerValue: null);
        var filter = new InternalAccessFilter();
        object? capturedResult = null;
        EndpointFilterDelegate next = _ => { capturedResult = "reached"; return ValueTask.FromResult<object?>("reached"); };

        // Act
        var result = await filter.InvokeAsync(ctx, next);

        // Assert
        capturedResult.Should().BeNull(because: "next delegate must NOT be called for non-loopback Production requests");
        // The filter returns Microsoft.AspNetCore.Http.Results.NotFound() which is an IResult.
        result.Should().BeAssignableTo<IResult>(because: "filter returns a 404 IResult");
        // Verify it is a NotFound result by executing it against the context's response.
        var httpCtx = ctx.HttpContext;
        await ((IResult)result!).ExecuteAsync(httpCtx);
        httpCtx.Response.StatusCode.Should().Be(StatusCodes.Status404NotFound,
            because: "non-loopback requests must return 404 to avoid fingerprinting the endpoint");
    }

    [Fact]
    public async Task Filter_allows_loopback_request_in_production()
    {
        var ctx = BuildContext(environment: "Production", remoteIp: "127.0.0.1", secret: null, headerValue: null);
        var filter = new InternalAccessFilter();
        object? capturedResult = null;
        EndpointFilterDelegate next = _ => { capturedResult = "reached"; return ValueTask.FromResult<object?>("reached"); };

        var result = await filter.InvokeAsync(ctx, next);

        capturedResult.Should().Be("reached", because: "loopback requests must be allowed through");
    }

    [Fact]
    public async Task Filter_allows_valid_X_Internal_Secret_in_production()
    {
        const string secret = "my-very-secret-value";
        var ctx = BuildContext(environment: "Production", remoteIp: "203.0.113.1", secret: secret, headerValue: secret);
        var filter = new InternalAccessFilter();
        object? capturedResult = null;
        EndpointFilterDelegate next = _ => { capturedResult = "reached"; return ValueTask.FromResult<object?>("reached"); };

        var result = await filter.InvokeAsync(ctx, next);

        capturedResult.Should().Be("reached", because: "correct X-Internal-Secret must be allowed");
    }

    [Fact]
    public async Task Filter_returns_404_for_wrong_X_Internal_Secret_in_production()
    {
        const string secret = "correct-secret";
        var ctx = BuildContext(environment: "Production", remoteIp: "203.0.113.1", secret: secret, headerValue: "wrong-secret");
        var filter = new InternalAccessFilter();
        object? capturedResult = null;
        EndpointFilterDelegate next = _ => { capturedResult = "reached"; return ValueTask.FromResult<object?>("reached"); };

        var result = await filter.InvokeAsync(ctx, next);

        capturedResult.Should().BeNull(because: "wrong X-Internal-Secret must not be allowed");
    }

    [Fact]
    public async Task Filter_allows_all_requests_in_testing_environment()
    {
        var ctx = BuildContext(environment: "Testing", remoteIp: "203.0.113.1", secret: null, headerValue: null);
        var filter = new InternalAccessFilter();
        object? capturedResult = null;
        EndpointFilterDelegate next = _ => { capturedResult = "reached"; return ValueTask.FromResult<object?>("reached"); };

        await filter.InvokeAsync(ctx, next);

        capturedResult.Should().Be("reached", because: "Testing environment bypasses the filter");
    }

    // ── Helpers ──────────────────────────────────────────────────────────────

    private static EndpointFilterInvocationContext BuildContext(
        string environment,
        string remoteIp,
        string? secret,
        string? headerValue)
    {
        var services = new ServiceCollection();
        services.AddLogging();

        // IWebHostEnvironment
        var env = new FakeWebHostEnvironment(environment);
        services.AddSingleton<IWebHostEnvironment>(env);

        // IConfiguration
        var configValues = new Dictionary<string, string?>();
        if (secret is not null)
            configValues["ApiTool:Internal:Secret"] = secret;
        var config = new ConfigurationBuilder().AddInMemoryCollection(configValues).Build();
        services.AddSingleton<IConfiguration>(config);

        var sp = services.BuildServiceProvider();

        var httpContext = new DefaultHttpContext { RequestServices = sp };
        httpContext.Connection.RemoteIpAddress = IPAddress.Parse(remoteIp);
        if (headerValue is not null)
            httpContext.Request.Headers["X-Internal-Secret"] = headerValue;

        return new TestEndpointFilterInvocationContext(httpContext);
    }

    private sealed class FakeWebHostEnvironment(string environmentName) : IWebHostEnvironment
    {
        public string EnvironmentName { get; set; } = environmentName;
        public string ApplicationName { get; set; } = "Test";
        public string WebRootPath { get; set; } = string.Empty;
        public Microsoft.Extensions.FileProviders.IFileProvider WebRootFileProvider { get; set; } =
            new Microsoft.Extensions.FileProviders.NullFileProvider();
        public string ContentRootPath { get; set; } = string.Empty;
        public Microsoft.Extensions.FileProviders.IFileProvider ContentRootFileProvider { get; set; } =
            new Microsoft.Extensions.FileProviders.NullFileProvider();
    }

    private sealed class TestEndpointFilterInvocationContext(HttpContext httpContext)
        : EndpointFilterInvocationContext
    {
        public override HttpContext HttpContext { get; } = httpContext;
        public override IList<object?> Arguments { get; } = [];
        public override T GetArgument<T>(int index) => throw new NotImplementedException();
    }
}
