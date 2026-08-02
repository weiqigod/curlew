using System.Text.RegularExpressions;
using Microsoft.Extensions.Configuration;

namespace ApiTool.Backend.Bootstrap;

/// <summary>Validated bootstrap credentials read from environment variables.</summary>
public sealed record AdminBootstrapConfig(string Email, string Password)
{
    /// <summary>Minimum acceptable password length for a bootstrap admin.</summary>
    public const int MinPasswordLength = 12;

    private static readonly Regex EmailRegex = new(
        @"^[^@\s]+@[^@\s]+\.[^@\s]+$",
        RegexOptions.Compiled | RegexOptions.IgnoreCase);

    /// <summary>
    /// Reads <c>BOOTSTRAP_ADMIN_EMAIL</c> and <c>BOOTSTRAP_ADMIN_PASSWORD</c> from configuration.
    /// Returns <c>(null, null)</c> when neither env var is set (bootstrap disabled),
    /// or a <see cref="BootstrapConfigError"/> when the pair is present but invalid.
    /// </summary>
    public static (AdminBootstrapConfig? Config, BootstrapConfigError? Error) FromConfiguration(
        IConfiguration cfg)
    {
        var email    = cfg["BOOTSTRAP_ADMIN_EMAIL"];
        var password = cfg["BOOTSTRAP_ADMIN_PASSWORD"];

        var emailSet    = !string.IsNullOrEmpty(email);
        var passwordSet = !string.IsNullOrEmpty(password);

        // Both unset → bootstrap disabled.
        if (!emailSet && !passwordSet)
            return (null, null);

        if (!emailSet)
            return (null, BootstrapConfigError.EmailMissing);
        if (!passwordSet)
            return (null, BootstrapConfigError.PasswordMissing);

        // Validate email format (basic structure check; lowercase before storing).
        var normalizedEmail = email!.ToLowerInvariant();
        if (!EmailRegex.IsMatch(normalizedEmail))
            return (null, BootstrapConfigError.EmailInvalid);

        if (password!.Length < MinPasswordLength)
            return (null, BootstrapConfigError.PasswordTooShort);

        return (new AdminBootstrapConfig(normalizedEmail, password), null);
    }
}

/// <summary>Describes why bootstrap configuration is invalid.</summary>
public enum BootstrapConfigError
{
    /// <summary>BOOTSTRAP_ADMIN_EMAIL is blank when BOOTSTRAP_ADMIN_PASSWORD is set.</summary>
    EmailMissing,

    /// <summary>BOOTSTRAP_ADMIN_PASSWORD is blank when BOOTSTRAP_ADMIN_EMAIL is set.</summary>
    PasswordMissing,

    /// <summary>BOOTSTRAP_ADMIN_PASSWORD is shorter than <see cref="AdminBootstrapConfig.MinPasswordLength"/> characters.</summary>
    PasswordTooShort,

    /// <summary>BOOTSTRAP_ADMIN_EMAIL is not a valid email address.</summary>
    EmailInvalid,
}
