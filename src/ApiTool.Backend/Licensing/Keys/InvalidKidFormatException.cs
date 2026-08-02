namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Thrown when an untrusted kid value fails the allowlist regex
/// (<see cref="KeyId.Allowlist"/>). Caught at the endpoint boundary and mapped
/// to HTTP 400 with <c>code: AUTH_INVALID_KID</c>.
/// </summary>
public sealed class InvalidKidFormatException(string kid)
    : Exception($"kid '{kid}' fails the allowlist regex ^[a-z0-9-]{{1,64}}$")
{
    /// <summary>The rejected kid value.</summary>
    public string Kid { get; } = kid;
}
