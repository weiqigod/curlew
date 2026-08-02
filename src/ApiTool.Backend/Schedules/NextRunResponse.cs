namespace ApiTool.Backend.Schedules;

/// <summary>Response body for <c>GET /api/v1/schedules/next-run</c>.</summary>
public sealed record NextRunResponse(
    /// <summary>The run identifier (prefixed <c>run_&lt;hex&gt;</c>).</summary>
    string RunId,

    /// <summary>The schedule identifier (prefixed <c>sched_&lt;hex&gt;</c>).</summary>
    string ScheduleId,

    /// <summary>The collection ref the worker should execute.</summary>
    string CollectionRef,

    /// <summary>Environment variables to inject for this run (currently empty; populated in a later milestone).</summary>
    IReadOnlyDictionary<string, string> EnvVars,

    /// <summary>The server-minted claim token the worker must present on every heartbeat and result submission.</summary>
    string ClaimToken,

    /// <summary>UTC deadline by which the worker must send its first heartbeat to keep the claim alive.</summary>
    DateTime Deadline);
