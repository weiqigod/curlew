using ApiTool.Backend.Audit;
using Microsoft.AspNetCore.Http;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>Unit tests for <see cref="AuditCaptureMiddleware"/>.</summary>
public sealed class AuditCaptureMiddlewareTests
{
    private static async Task<AuditContext> InvokeAsync(
        string? forwardedFor = null,
        string? userAgent = null,
        System.Net.IPAddress? remoteIp = null)
    {
        var ctx = new DefaultHttpContext();
        if (forwardedFor is not null)
            ctx.Request.Headers["X-Forwarded-For"] = forwardedFor;
        if (userAgent is not null)
            ctx.Request.Headers.UserAgent = userAgent;
        if (remoteIp is not null)
            ctx.Connection.RemoteIpAddress = remoteIp;

        var auditContext = new AuditContext();
        var mw = new AuditCaptureMiddleware(_ => Task.CompletedTask);
        await mw.InvokeAsync(ctx, auditContext);
        return auditContext;
    }

    [Fact]
    public async Task Middleware_extracts_first_ip_from_x_forwarded_for()
    {
        var audit = await InvokeAsync(forwardedFor: "203.0.113.5, 10.0.0.1");
        audit.IpAddress.Should().Be("203.0.113.5");
    }

    [Fact]
    public async Task Middleware_falls_back_to_socket_peer_when_no_forwarded_header()
    {
        var ip = System.Net.IPAddress.Parse("198.51.100.1");
        var audit = await InvokeAsync(remoteIp: ip);
        audit.IpAddress.Should().Be("198.51.100.1");
    }

    [Fact]
    public async Task Middleware_stores_user_agent_header()
    {
        var audit = await InvokeAsync(userAgent: "curl/8.1");
        audit.UserAgent.Should().Be("curl/8.1");
    }

    [Fact]
    public async Task Middleware_truncates_user_agent_at_500_chars()
    {
        var longAgent = new string('A', 600);
        var audit = await InvokeAsync(userAgent: longAgent);
        audit.UserAgent.Should().HaveLength(500);
    }

    [Fact]
    public async Task Middleware_sets_null_ip_when_no_ip_available()
    {
        var audit = await InvokeAsync();
        audit.IpAddress.Should().BeNull();
    }

    [Fact]
    public async Task Middleware_stores_empty_string_for_missing_user_agent()
    {
        var audit = await InvokeAsync();
        audit.UserAgent.Should().Be(string.Empty);
    }

    [Fact]
    public async Task Middleware_calls_next_delegate()
    {
        var ctx = new DefaultHttpContext();
        var auditContext = new AuditContext();
        var nextCalled = false;
        var mw = new AuditCaptureMiddleware(_ =>
        {
            nextCalled = true;
            return Task.CompletedTask;
        });

        await mw.InvokeAsync(ctx, auditContext);

        nextCalled.Should().BeTrue();
    }
}
