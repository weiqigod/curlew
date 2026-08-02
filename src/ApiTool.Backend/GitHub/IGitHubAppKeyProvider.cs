// Refs docs/SPECIFICATION.md:8350-8388 (GitHub App Credentials and Custody).
// RS256 lives ONLY behind this interface (Decision #13, :8359).
// GitHub Enterprise Server is NOT supported (Open Decision #7, :8696). The api base
// is hardcoded to https://api.github.com by callers — this provider is purely a
// signing primitive.
namespace ApiTool.Backend.GitHub;

/// <summary>
/// Mints RS256 App JWTs for authenticating to GitHub's App-authentication endpoint.
/// RS256 is mandated by GitHub; this is the ONLY place RS256 lives in the system
/// (cf. <see cref="ApiTool.Backend.Licensing.Keys.IKeyProvider"/> for ES256).
/// JWTs are never persisted (file/DB/log/error response) — they live in memory
/// only for the duration of the installation-token exchange.
/// </summary>
public interface IGitHubAppKeyProvider
{
    /// <summary>
    /// Signs an App JWT with the configured private key. Returns the compact JWS string.
    /// Throws <see cref="InvalidAppIdMismatchException"/> when <paramref name="claims"/>.AppId
    /// does not equal <see cref="GetAppId"/> (per-product separation, :8387).
    /// Throws <see cref="GitHubAppJwtSigningException"/> on any signing failure
    /// (PEM unreadable, KMS unreachable, etc.) without leaking key material into
    /// the exception message.
    /// </summary>
    Task<string> SignAppJwtAsync(GitHubAppJwtClaims claims, CancellationToken ct = default);

    /// <summary>Returns the configured numeric App ID.</summary>
    long GetAppId();
}

/// <summary>
/// JWT claims for a GitHub App authentication JWT. Shape is fixed by GitHub
/// (header alg=RS256, typ=JWT; payload {iss, iat, exp}).
/// </summary>
public sealed record GitHubAppJwtClaims(long AppId, DateTimeOffset Iat, DateTimeOffset Exp);

/// <summary>Thrown when SignAppJwtAsync is called with an AppId different from the configured one.</summary>
public sealed class InvalidAppIdMismatchException(long requested, long configured)
    : InvalidOperationException(
        $"GitHub App id mismatch: caller requested {requested} but provider is configured for {configured}");

/// <summary>
/// Thrown when signing fails for any reason. The inner exception is preserved for
/// telemetry but its message is NOT included in this exception's user-visible message,
/// so PEM bytes / KMS resource names cannot leak into error responses or logs.
/// </summary>
public sealed class GitHubAppJwtSigningException : Exception
{
    /// <summary>
    /// Initialises a new <see cref="GitHubAppJwtSigningException"/> with a fixed provider-kind
    /// message. The inner exception is preserved for telemetry only.
    /// </summary>
    public GitHubAppJwtSigningException(string providerKind, Exception inner)
        : base($"GitHub App JWT signing failed (provider={providerKind})", inner) { }
}
