namespace ApiTool.Backend.Auth;

/// <summary>
/// Server-side password-strength scorer. Returns a 0-4 integer (zxcvbn convention).
/// The minimum acceptable score for password reset is 3 per spec :8531.
/// </summary>
public interface IPasswordStrengthChecker
{
    /// <summary>
    /// Scores <paramref name="password"/> on the zxcvbn 0-4 scale.
    /// <paramref name="userInputs"/> are domain-specific tokens (e.g. the user's email)
    /// that should be penalised if present in the password.
    /// </summary>
    int Score(string password, IEnumerable<string> userInputs);
}
