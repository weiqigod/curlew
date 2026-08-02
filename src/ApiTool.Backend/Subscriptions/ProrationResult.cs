namespace ApiTool.Backend.Subscriptions;

/// <summary>Result of a subscription proration calculation (amounts in cents).</summary>
/// <param name="Credit">Credit amount in cents for unused time on the old plan.</param>
/// <param name="Charge">Charge amount in cents for the new plan's remaining time.</param>
/// <param name="Net">Net amount due now (<c>Charge - Credit</c>) in cents.</param>
public sealed record ProrationResult(int Credit, int Charge, int Net);
