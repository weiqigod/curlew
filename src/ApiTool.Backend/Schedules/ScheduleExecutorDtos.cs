using ApiTool.Backend.Results;

namespace ApiTool.Backend.Schedules;

/// <summary>Request body for <c>POST /api/v1/schedules/runs/{run_id}/heartbeat</c>.</summary>
public sealed class ScheduleHeartbeatRequest
{
    /// <summary>The claim token issued when this run was claimed.</summary>
    public string? ClaimToken { get; set; }
}

/// <summary>Request body for <c>POST /api/v1/schedules/runs/{run_id}/result</c>.</summary>
public sealed class ScheduleResultRequest
{
    /// <summary>The claim token issued when this run was claimed.</summary>
    public string? ClaimToken { get; set; }
    /// <summary>Test collection name recorded on the result row.</summary>
    public string? CollectionName { get; set; }
    /// <summary>UTC time the run started.</summary>
    public DateTime? RunAt { get; set; }
    /// <summary>Total run duration in milliseconds.</summary>
    public long? DurationMs { get; set; }
    /// <summary>Number of passing tests.</summary>
    public int? PassCount { get; set; }
    /// <summary>Number of failing tests (determines pass/fail status).</summary>
    public int? FailCount { get; set; }
    /// <summary>Number of skipped tests.</summary>
    public int? SkippedCount { get; set; }
    /// <summary>Trigger source (defaults to "schedule" when null).</summary>
    public string? TriggeredBy { get; set; }
    /// <summary>Optional git SHA of the collection being tested.</summary>
    public string? GitSha { get; set; }
    /// <summary>Per-test result items.</summary>
    public IReadOnlyList<UploadResultItemRequest>? Items { get; set; }
}
