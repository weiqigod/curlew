namespace ApiTool.Backend.Bootstrap;

/// <summary>Exit codes used when the application terminates during startup bootstrap.</summary>
public static class BootstrapExitCodes
{
    /// <summary>Bootstrap configuration is invalid (e.g., password too short, missing email).</summary>
    public const int BootstrapInvalid = 3;

    /// <summary>EF Core migration failed (e.g., DB unreachable, SQL error).</summary>
    public const int MigrationsFailed = 4;
}
