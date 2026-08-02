using ApiTool.Backend.Notifications.Trials;
using ApiTool.Backend.Tests.TestInfrastructure;
using FluentAssertions;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Options;

namespace ApiTool.Backend.Tests.Notifications.Trials;

[Collection(BackendCollection.Name)]
public sealed class TrialExpiryNotifierWiringTests
{
    private readonly BackendFactory _factory;

    public TrialExpiryNotifierWiringTests(BackendFactory f) => _factory = f;

    [Fact]
    public void Options_bind_with_section_key_and_defaults()
    {
        using var scope = _factory.Services.CreateScope();
        var opts = scope.ServiceProvider
            .GetRequiredService<IOptions<TrialExpiryNotifierOptions>>().Value;
        opts.RunAtUtc.Should().Be("09:00");
        opts.BatchSize.Should().Be(500);
    }

    [Fact]
    public void Hosted_service_is_not_registered_in_Testing()
    {
        // Sanity: in BackendFactory we deliberately skip the AddHostedService
        // gate. This guards the gate from being accidentally removed.
        var hosted = _factory.Services
            .GetServices<Microsoft.Extensions.Hosting.IHostedService>()
            .OfType<TrialExpiryNotifier>()
            .ToList();
        hosted.Should().BeEmpty();
    }
}
