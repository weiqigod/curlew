using ApiTool.Backend.PrChecks;

namespace ApiTool.Backend.Tests.TestInfrastructure;

/// <summary>
/// A fake <see cref="IGitLabCheckPoster"/> for endpoint integration tests.
/// Returns a pre-configured <see cref="CheckRunPostResult"/> without making any HTTP calls.
/// </summary>
public sealed class FakeGitLabCheckPoster : IGitLabCheckPoster
{
    private CheckRunPostResult _result;

    /// <summary>Gets the ID of the last <c>pr_checks</c> row passed to <see cref="PostAsync"/>.</summary>
    public Guid? LastPostId { get; private set; }

    /// <summary>Number of times <see cref="PostAsync"/> was called.</summary>
    public int CallCount { get; private set; }

    public FakeGitLabCheckPoster(CheckRunPostResult result)
    {
        _result = result;
    }

    /// <summary>Creates a fake that returns a successful <c>posted</c> result.</summary>
    public static FakeGitLabCheckPoster Posted() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Posted, null, null, PrCheckErrorCode.None));

    /// <summary>Creates a fake that returns a <c>Failed / GitLabNoInstallation</c> result.</summary>
    public static FakeGitLabCheckPoster NoInstallation() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Failed, null,
            "No GitLab installation found.", PrCheckErrorCode.GitLabNoInstallation));

    /// <summary>Creates a fake that returns a <c>Failed / GitLabTokenRevoked</c> result.</summary>
    public static FakeGitLabCheckPoster TokenRevoked() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Failed, null,
            "GitLab PAT revoked.", PrCheckErrorCode.GitLabTokenRevoked));

    /// <summary>Creates a fake that returns a <c>Queued / GitLabRateLimited</c> result.</summary>
    public static FakeGitLabCheckPoster RateLimited() =>
        new(new CheckRunPostResult(PrCheckPostStatus.Queued, null,
            "GitLab rate-limited.", PrCheckErrorCode.GitLabRateLimited));

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
