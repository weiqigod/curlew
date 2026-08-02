using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Internal.TierGates;

namespace ApiTool.Backend.Tests.Internal.TierGates;

/// <summary>
/// Verifies that each per-feature tier-gate adapter delegates to <see cref="ITierGate"/>
/// with the correct <see cref="SubscriptionTier"/> minimum and translates all
/// <see cref="TierGateResult"/> variants into the corresponding per-feature error enum value.
/// </summary>
public class AdapterDelegationTests
{
    private sealed class FakeTierGate(TierGateResult result) : ITierGate
    {
        public Guid LastOrgId { get; private set; }
        public SubscriptionTier LastRequired { get; private set; }

        public Task<TierGateResult> EnsureAsync(Guid orgId, SubscriptionTier required, CancellationToken ct)
        {
            LastOrgId = orgId;
            LastRequired = required;
            return Task.FromResult(result);
        }
    }

    // --- VaultConfigTierGate ---

    [Theory]
    [InlineData(TierGateResult.Allowed,        VaultConfigError.None)]
    [InlineData(TierGateResult.OrgNotFound,    VaultConfigError.OrgNotFound)]
    [InlineData(TierGateResult.TierIneligible, VaultConfigError.TierIneligible)]
    public async Task VaultConfigTierGate_translates_all_results(TierGateResult inner, VaultConfigError expected)
    {
        var fake = new FakeTierGate(inner);
        var orgId = Guid.NewGuid();

        var result = await VaultConfigTierGate.EnsureTeamOrAboveAsync(fake, orgId, default);

        result.Should().Be(expected);
        fake.LastOrgId.Should().Be(orgId);
        fake.LastRequired.Should().Be(SubscriptionTier.Team);
    }

    // --- ScheduleExecutorTierGate ---

    [Theory]
    [InlineData(TierGateResult.Allowed,        ScheduleExecutorError.None)]
    [InlineData(TierGateResult.OrgNotFound,    ScheduleExecutorError.OrgNotFound)]
    [InlineData(TierGateResult.TierIneligible, ScheduleExecutorError.TierIneligible)]
    public async Task ScheduleExecutorTierGate_translates_all_results(TierGateResult inner, ScheduleExecutorError expected)
    {
        var fake = new FakeTierGate(inner);
        var orgId = Guid.NewGuid();

        var result = await ScheduleExecutorTierGate.EnsureTeamOrAboveAsync(fake, orgId, default);

        result.Should().Be(expected);
        fake.LastOrgId.Should().Be(orgId);
        fake.LastRequired.Should().Be(SubscriptionTier.Team);
    }

    // --- DashboardTierGate ---

    [Theory]
    [InlineData(TierGateResult.Allowed,        DashboardError.None)]
    [InlineData(TierGateResult.OrgNotFound,    DashboardError.OrgNotFound)]
    [InlineData(TierGateResult.TierIneligible, DashboardError.TierIneligible)]
    public async Task DashboardTierGate_translates_all_results(TierGateResult inner, DashboardError expected)
    {
        var fake = new FakeTierGate(inner);
        var orgId = Guid.NewGuid();

        var result = await DashboardTierGate.EnsureTeamOrAboveAsync(fake, orgId, default);

        result.Should().Be(expected);
        fake.LastOrgId.Should().Be(orgId);
        fake.LastRequired.Should().Be(SubscriptionTier.Team);
    }

    // --- AuditLogExportTierGate ---

    [Theory]
    [InlineData(TierGateResult.Allowed,        AuditLogExportError.None)]
    [InlineData(TierGateResult.OrgNotFound,    AuditLogExportError.OrgNotFound)]
    [InlineData(TierGateResult.TierIneligible, AuditLogExportError.TierIneligible)]
    public async Task AuditLogExportTierGate_translates_all_results(TierGateResult inner, AuditLogExportError expected)
    {
        var fake = new FakeTierGate(inner);
        var orgId = Guid.NewGuid();

        var result = await AuditLogExportTierGate.EnsureEnterpriseAsync(fake, orgId, default);

        result.Should().Be(expected);
        fake.LastOrgId.Should().Be(orgId);
        fake.LastRequired.Should().Be(SubscriptionTier.Enterprise);
    }
}
