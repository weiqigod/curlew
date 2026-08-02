using ApiTool.Backend.PrChecks;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A fake <see cref="ICheckRunPoster"/> for endpoint integration tests.
/// Returns a pre-configured <see cref="CheckRunPostResult"/> without making any HTTP calls.
/// </summary>
public sealed class FakeCheckRunPoster : ICheckRunPoster
{
    private CheckRunPostResult _result;

    /// <summary>Gets the ID of the last <c>pr_checks</c> row passed to <see cref="PostAsync"/>.</summary>
    public Guid? LastPostId { get; private set; }

    /// <summary>Number of times <see cref="PostAsync"/> was called.</summary>
    public int CallCount { get; private set; }

    public FakeCheckRunPoster(CheckRunPostResult result)
    {
        _result = result;
    }

    /// <summary>Creates a fake that returns a successful <c>posted</c> result with the given check_run_id.</summary>
    public static FakeCheckRunPoster Posted(long checkRunId = 42L) =>
        new(new CheckRunPostResult(PrCheckPostStatus.Posted, checkRunId, null, null));

    /// <summary>Creates a fake that returns a <c>Failed / NoInstallation</c> result.</summary>
    public static FakeCheckRunPoster NoInstallation() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Failed, null,
            "No GitHub App installation found.", PrCheckErrorCode.NoInstallation));

    /// <summary>Creates a fake that returns a <c>Failed / RepoNotCovered</c> result.</summary>
    public static FakeCheckRunPoster RepoNotCovered() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Failed, null,
            "Repo not covered.", PrCheckErrorCode.RepoNotCovered));

    /// <summary>Creates a fake that returns a <c>Queued / GithubRateLimited</c> result.</summary>
    public static FakeCheckRunPoster RateLimited() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Queued, null,
            "Rate-limited.", PrCheckErrorCode.GithubRateLimited));

    /// <summary>Updates the result returned by subsequent <see cref="PostAsync"/> calls.</summary>
    public void SetResult(CheckRunPostResult result) => _result = result;

    /// <inheritdoc/>
    public Task<CheckRunPostResult> PostAsync(Guid prCheckId, CancellationToken ct)
    {
        LastPostId = prCheckId;
        CallCount++;
        return Task.FromResult(_result);
    }
}
