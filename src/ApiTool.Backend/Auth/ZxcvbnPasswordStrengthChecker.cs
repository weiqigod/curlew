namespace ApiTool.Backend.Auth;

/// <summary>
/// <see cref="IPasswordStrengthChecker"/> implementation backed by the zxcvbn-core library.
/// Delegates scoring to <c>Zxcvbn.Core.EvaluatePassword</c>.
/// </summary>
public sealed class ZxcvbnPasswordStrengthChecker : IPasswordStrengthChecker
{
    /// <inheritdoc/>
    public int Score(string password, IEnumerable<string> userInputs)
    {
        ArgumentNullException.ThrowIfNull(password);
        ArgumentNullException.ThrowIfNull(userInputs);
        var inputs = userInputs as IList<string> ?? [.. userInputs];
        var result = Zxcvbn.Core.EvaluatePassword(password, inputs);
        return result.Score;
    }
}
