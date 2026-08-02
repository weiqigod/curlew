namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// The lifecycle state of a <c>signing_keys</c> row.
/// Refs docs/SPECIFICATION.md:9715-9735.
/// </summary>
public static class KeyStatus
{
    /// <summary>The key currently used for signing new tokens.</summary>
    public const string Current = "current";

    /// <summary>The key staged for the next rotation.</summary>
    public const string Next = "next";

    /// <summary>A retired key still accepted for token verification during the overlap window.</summary>
    public const string Verifying = "verifying";

    /// <summary>A key that has been revoked and must not be used for signing or verification.</summary>
    public const string Revoked = "revoked";
}
