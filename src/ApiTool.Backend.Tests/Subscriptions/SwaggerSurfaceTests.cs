using System.Text.Json;
using ApiTool.Backend.Tests.TestInfrastructure;

namespace ApiTool.Backend.Tests.Subscriptions;

/// <summary>Asserts that Swagger lists all required subscription, invitation, and member endpoints.</summary>
[Collection(BackendCollection.Name)]
public sealed class SwaggerSurfaceTests : IAsyncLifetime
{
    private readonly BackendFactory _factory;

    public SwaggerSurfaceTests(BackendFactory factory) => _factory = factory;

    public async Task InitializeAsync() => await _factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task Swagger_lists_all_subscription_invitation_and_member_endpoints()
    {
        var client = _factory.CreateClient();

        var json = await client.GetStringAsync("/swagger/v1/swagger.json");
        using var doc = JsonDocument.Parse(json);
        var paths = doc.RootElement.GetProperty("paths");

        // Subscription endpoints (7 from spec lines 6738–6891, plus M14-009 billing-portal).
        paths.TryGetProperty("/api/v1/subscriptions/checkout", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions/{id}", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions/{id}/reactivate", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions/portal", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions/billing-portal", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/subscriptions/preview-proration", out _).Should().BeTrue();

        // Invitation endpoints (5 from spec lines 7215–7234).
        paths.TryGetProperty("/api/v1/organizations/{id}/invitations", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/invitations/{inviteId}", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/invitations/{inviteId}/resend", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/invitations/accept", out _).Should().BeTrue();

        // Member management endpoints.
        paths.TryGetProperty("/api/v1/organizations/{id}/members", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/members/{memberId}", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/transfer", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/leave", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}", out _).Should().BeTrue();
        paths.TryGetProperty("/api/v1/organizations/{id}/cancel-deletion", out _).Should().BeTrue();
    }
}
