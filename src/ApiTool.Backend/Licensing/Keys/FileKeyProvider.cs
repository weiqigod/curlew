// Refs docs/SPECIFICATION.md:8040, 8060 (FileKeyProvider — self-hosted key custody).
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// File-system–backed <see cref="IKeyProvider"/> for self-hosted deployments.
/// Private key material is stored as PKCS#8 PEM files in <c>&lt;Dir&gt;/&lt;kid&gt;.pem</c>
/// with permissions enforced to 0600 on Unix. Signing uses RFC 6979 deterministic
/// ECDSA via BouncyCastle (see <see cref="BouncyCastleEs256"/>).
/// Refs docs/SPECIFICATION.md:8040, 8060.
/// </summary>
public sealed class FileKeyProvider(
    ISigningKeyStore store,
    IOptions<KeyProviderOptions> opts,
    TimeProvider clock,
    ILogger<FileKeyProvider> logger) : IKeyProvider
{
    private readonly string _dir = opts.Value.File.Dir;
    private readonly string _env = opts.Value.Env;

    /// <inheritdoc/>
    public async Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default)
    {
        var current = await store.LoadCurrentAsync(ct) ?? await BootstrapAsync(ct);
        var pem = await ReadPrivatePemAsync(current.Kid, ct);
        var sig = BouncyCastleEs256.SignDeterministic(pem, payload);
        return new SignatureResult(current.Kid, sig);
    }

    /// <inheritdoc/>
    public async Task<string> GetActiveKidAsync(CancellationToken ct = default)
    {
        var current = await store.LoadCurrentAsync(ct) ?? await BootstrapAsync(ct);
        return current.Kid;
    }

    /// <inheritdoc/>
    public async Task<JsonWebKeySet> GetVerificationJwksAsync(CancellationToken ct = default)
    {
        var rows = await store.LoadCurrentAndVerifyingAsync(ct);
        var jwks = new JsonWebKeySet();
        foreach (var row in rows)
        {
            try
            {
                var jwk = JsonSerializer.Deserialize<JsonWebKey>(row.PublicKeyJwkJson);
                if (jwk is not null)
                    jwks.Keys.Add(jwk);
            }
            catch (JsonException ex)
            {
                logger.LogWarning(ex, "signing-key: malformed public_key_jwk for kid={Kid}", row.Kid);
            }
        }
        return jwks;
    }

    /// <summary>
    /// Validates the kid format and returns the private key PEM.
    /// Throws <see cref="InvalidKidFormatException"/> for invalid kids — no DB lookup occurs.
    /// </summary>
    /// <exception cref="InvalidKidFormatException">When <paramref name="kid"/> fails the allowlist.</exception>
    public async Task<string> GetKeyForSigningAsync(string kid, CancellationToken ct = default)
    {
        if (!KeyId.IsValid(kid))
            throw new InvalidKidFormatException(kid);

        return await ReadPrivatePemAsync(kid, ct);
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    private async Task<SigningKey> BootstrapAsync(CancellationToken ct)
    {
        // Re-check under the assumption that a concurrent request may have just bootstrapped.
        var existing = await store.LoadCurrentAsync(ct);
        if (existing is not null)
            return existing;

        var kid = KeyId.Generate(_env, "es256", clock);
        var (pem, jwk) = BouncyCastleEs256.GenerateKeyPair(kid);

        await WritePrivatePemAsync(kid, pem, ct);

        var row = new SigningKey
        {
            Kid = kid,
            Algorithm = "ES256",
            Status = KeyStatus.Current,
            PublicKeyJwkJson = JsonSerializer.Serialize(jwk),
            CreatedAt = clock.GetUtcNow().UtcDateTime,
            PromotedAt = clock.GetUtcNow().UtcDateTime,
        };

        try
        {
            await store.InsertAsync(row, ct);
        }
        catch (Microsoft.EntityFrameworkCore.DbUpdateException)
        {
            // A concurrent bootstrap already inserted a current key — re-read it.
            logger.LogDebug("signing-key: concurrent bootstrap detected for env={Env}, re-reading current key", _env);
            return await store.LoadCurrentAsync(ct)
                ?? throw new InvalidOperationException("signing-key: concurrent bootstrap succeeded but LoadCurrentAsync returned null");
        }

        logger.LogInformation("signing-key: bootstrapped current kid={Kid}", kid);
        return row;
    }

    private async Task WritePrivatePemAsync(string kid, string pem, CancellationToken ct)
    {
        EnsureDirectoryExists();
        var path = PemPath(kid);
        await System.IO.File.WriteAllTextAsync(path, pem, ct);

        if (OperatingSystem.IsLinux() || OperatingSystem.IsMacOS())
        {
            System.IO.File.SetUnixFileMode(path, UnixFileMode.UserRead | UnixFileMode.UserWrite);
        }
    }

    private async Task<string> ReadPrivatePemAsync(string kid, CancellationToken ct, bool isRetry = false)
    {
        var path = PemPath(kid);
        if (!System.IO.File.Exists(path))
        {
            if (isRetry)
                throw new InvalidOperationException(
                    $"signing-key: PEM file for kid={kid} is missing even after re-bootstrap. " +
                    "Check disk space and key directory permissions.");

            // DB says current exists but file is missing — re-bootstrap once.
            logger.LogWarning("signing-key: db says kid={Kid} but file missing — re-bootstrapping", kid);
            var reborn = await BootstrapAsync(ct);
            return await ReadPrivatePemAsync(reborn.Kid, ct, isRetry: true);
        }
        return await System.IO.File.ReadAllTextAsync(path, ct);
    }

    private void EnsureDirectoryExists()
    {
        if (!Directory.Exists(_dir))
            Directory.CreateDirectory(_dir);
    }

    private string PemPath(string kid) => Path.Combine(_dir, $"{kid}.pem");
}
