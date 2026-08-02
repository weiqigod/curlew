using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;

namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Unit tests for <see cref="AuditLogExportTierGate"/> — verifies that the adapter
/// delegates to <see cref="ITierGate"/> with <see cref="SubscriptionTier.Enterprise"/>
/// and maps all <see cref="TierGateResult"/> values to <see cref="AuditLogExportError"/>.
/// </summary>
public sealed class AuditLogExportTierGateTests
{
    private sealed class StubGate(TierGateResult result) : ITierGate
    {
        public SubscriptionTier LastRequired { get; private set; }

        public Task<TierGateResult> EnsureAsync(Guid orgId, SubscriptionTier required, CancellationToken ct)
        {
            LastRequired = required;
            return Task.FromResult(result);
        }
    }

    [Theory]
    [InlineData(TierGateResult.Allowed,        AuditLogExportError.None)]
    [InlineData(TierGateResult.OrgNotFound,    AuditLogExportError.OrgNotFound)]
    [InlineData(TierGateResult.TierIneligible, AuditLogExportError.TierIneligible)]
    public async Task EnsureEnterpriseAsync_maps_result_to_error(
        TierGateResult gateResult, AuditLogExportError expected)
    {
        var stub = new StubGate(gateResult);
        var got = await AuditLogExportTierGate.EnsureEnterpriseAsync(
            stub, Guid.NewGuid(), CancellationToken.None);
        got.Should().Be(expected);
    }

    [Fact]
    public async Task EnsureEnterpriseAsync_delegates_with_Enterprise_tier()
    {
        var stub = new StubGate(TierGateResult.Allowed);
        await AuditLogExportTierGate.EnsureEnterpriseAsync(stub, Guid.NewGuid(), CancellationToken.None);
        stub.LastRequired.Should().Be(SubscriptionTier.Enterprise);
    }

    [Fact]
    public void RequiredMinimum_is_Enterprise()
        => AuditLogExportTierGate.RequiredMinimum.Should().Be(SubscriptionTier.Enterprise);
}
