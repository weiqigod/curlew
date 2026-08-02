using System.Net.Sockets;
using System.Text;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Health;

/// <summary>Redis health probe using a raw TCP PING/PONG round-trip. No NuGet dependencies.</summary>
public sealed class TcpRedisHealthProbe(IConfiguration config) : IRedisHealthProbe
{
    private readonly string? _host = config["REDIS_HOST"];
    private readonly int _port = int.TryParse(config["REDIS_PORT"], out var p) ? p : 6379;

    /// <inheritdoc/>
    public bool IsConfigured => !string.IsNullOrWhiteSpace(_host);

    /// <inheritdoc/>
    public async Task<bool> IsConnectedAsync(CancellationToken ct)
    {
        if (!IsConfigured) return false;

        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(TimeSpan.FromSeconds(2));
        using var client = new TcpClient { NoDelay = true };
        try
        {
            await client.ConnectAsync(_host!, _port, cts.Token);
            await using var stream = client.GetStream();
            // RESP inline command: "PING\r\n" → "+PONG\r\n"
            var cmd = Encoding.ASCII.GetBytes("PING\r\n");
            await stream.WriteAsync(cmd, cts.Token);
            var buf = new byte[7]; // "+PONG\r\n"
            var read = await stream.ReadAsync(buf.AsMemory(), cts.Token);
            var reply = Encoding.ASCII.GetString(buf, 0, read);
            return reply.StartsWith("+PONG", StringComparison.Ordinal);
        }
        catch (OperationCanceledException)
        {
            throw;
        }
        catch
        {
            return false;
        }
    }
}
