namespace ApiTool.Backend.Data.Entities;

/// <summary>Outcome of an individual test case.</summary>
public enum ResultStatus
{
    /// <summary>Test case passed.</summary>
    Passed,

    /// <summary>Test case failed.</summary>
    Failed,

    /// <summary>Test case was skipped.</summary>
    Skipped,

    /// <summary>Test case encountered an unexpected error.</summary>
    Error,
}
