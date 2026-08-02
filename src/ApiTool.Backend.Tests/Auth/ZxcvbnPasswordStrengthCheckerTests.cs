using ApiTool.Backend.Auth;

namespace ApiTool.Backend.Tests.Auth;

/// <summary>Unit tests for <see cref="ZxcvbnPasswordStrengthChecker"/>.</summary>
public sealed class ZxcvbnPasswordStrengthCheckerTests
{
    [Theory]
    [InlineData("password",  2)]  // common word — score 0
    [InlineData("Password1!", 2)]  // common variant — still weak
    public void Weak_passwords_score_below_three(string password, int upperBoundInclusive)
    {
        var checker = new ZxcvbnPasswordStrengthChecker();
        checker.Score(password, []).Should().BeLessThanOrEqualTo(upperBoundInclusive);
    }

    [Fact]
    public void Strong_passphrase_scores_three_or_higher()
    {
        var checker = new ZxcvbnPasswordStrengthChecker();
        checker.Score("correct horse battery staple", []).Should().BeGreaterThanOrEqualTo(3);
    }

    [Fact]
    public void Email_in_password_is_penalised()
    {
        var checker = new ZxcvbnPasswordStrengthChecker();
        var withEmail    = checker.Score("alex@example.com!23", ["alex@example.com"]);
        var withoutEmail = checker.Score("alex@example.com!23", []);
        withEmail.Should().BeLessThanOrEqualTo(withoutEmail);
    }
}
