using System.Security.Cryptography;
using System.Text;
using Konscious.Security.Cryptography;

namespace ApiTool.Backend.Auth;

/// <summary>Argon2id password hasher that emits and parses PHC-format strings.</summary>
public sealed class PasswordHasher
{
    private const int SaltBytes = 16;
    private const int HashBytes = 32;
    private const int MemoryKiB = 65536;   // 64 MiB — OWASP 2024 recommended
    private const int Iterations = 3;
    private const int Parallelism = 4;

    /// <summary>Hashes a plaintext password and returns an OWASP-style PHC encoded string.</summary>
    public string Hash(string password)
    {
        ArgumentException.ThrowIfNullOrEmpty(password);
        var salt = RandomNumberGenerator.GetBytes(SaltBytes);
        var hash = ComputeHash(password, salt);
        return $"$argon2id$v=19$m={MemoryKiB},t={Iterations},p={Parallelism}$" +
               $"{Convert.ToBase64String(salt)}${Convert.ToBase64String(hash)}";
    }

    /// <summary>
    /// Verifies a plaintext password against a PHC-encoded hash. Returns
    /// <see langword="false"/> on any malformed input instead of throwing.
    /// </summary>
    public bool Verify(string password, string encoded)
    {
        if (string.IsNullOrEmpty(password) || string.IsNullOrEmpty(encoded))
            return false;
        if (!TryParsePhc(encoded, out var parsed)) return false;
        var computed = ComputeHash(password, parsed.Salt, parsed.MemoryKiB, parsed.Iterations, parsed.Parallelism, parsed.Hash.Length);
        return CryptographicOperations.FixedTimeEquals(computed, parsed.Hash);
    }

    private static byte[] ComputeHash(
        string password, byte[] salt,
        int memoryKiB = MemoryKiB, int iterations = Iterations,
        int parallelism = Parallelism, int hashBytes = HashBytes)
    {
        using var argon = new Argon2id(Encoding.UTF8.GetBytes(password))
        {
            Salt = salt,
            MemorySize = memoryKiB,
            Iterations = iterations,
            DegreeOfParallelism = parallelism,
        };
        return argon.GetBytes(hashBytes);
    }

    private record ParsedPhc(byte[] Salt, byte[] Hash, int MemoryKiB, int Iterations, int Parallelism);

    private static bool TryParsePhc(string encoded, out ParsedPhc parsed)
    {
        parsed = null!;
        // Expected format: $argon2id$v=19$m=65536,t=3,p=4$<salt-b64>$<hash-b64>
        var parts = encoded.Split('$', StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length != 5 || parts[0] != "argon2id") return false;
        if (parts[1] != "v=19") return false;
        var paramPairs = parts[2].Split(',');
        int m = 0, t = 0, p = 0;
        foreach (var pair in paramPairs)
        {
            var kv = pair.Split('=');
            if (kv.Length != 2) return false;
            if (!int.TryParse(kv[1], out var n)) return false;
            switch (kv[0]) { case "m": m = n; break; case "t": t = n; break; case "p": p = n; break; }
        }
        if (m <= 0 || t <= 0 || p <= 0) return false;
        try
        {
            parsed = new ParsedPhc(Convert.FromBase64String(parts[3]), Convert.FromBase64String(parts[4]), m, t, p);
            return true;
        }
        catch (FormatException) { return false; }
    }
}
