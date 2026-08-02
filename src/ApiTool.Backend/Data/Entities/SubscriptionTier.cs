namespace ApiTool.Backend.Data.Entities;

/// <summary>The pricing tier of an organization's subscription.</summary>
public enum SubscriptionTier
{
    /// <summary>Free tier — owner only, 1 seat.</summary>
    Free,

    /// <summary>Professional tier — individual paid plan.</summary>
    Professional,

    /// <summary>Team tier — collaborative plan.</summary>
    Team,

    /// <summary>Enterprise tier — large organization plan.</summary>
    Enterprise,
}
