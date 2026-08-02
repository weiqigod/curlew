// Refs docs/SPECIFICATION.md:8040, 8060 (GoogleKmsKeyProvider — SaaS key custody).
using System.Text.Json;
using ApiTool.Backend.Data.Entities;
using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Options;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Google Cloud KMS–backed <see cref="IKeyProvider"/> for SaaS deployments.
/// Key material is held by KMS (HSM tier, EC_SIGN_P256_SHA256); only the public key
/// is cached in <c>signing_keys.public_key_jwk</c> for JWKS responses.
/// KMS path is inherently deterministic by contract (RFC 6979-compliant at KMS level).
/// Refs docs/SPECIFICATION.md:8040, 8060.
/// </summary>
public sealed class GoogleKmsKeyProvider(
    IKmsClient kms,
    ISigningKeyStore store,
    IOptions<KeyProviderOptions> opts,
    TimeProvider clock,
    ILogger<GoogleKmsKeyProvider> logger) : IKeyProvider
{
    private readonly string _env = opts.Value.Env;
    private readonly KeyProviderOptions.KmsProviderOptions _kmsOpts = opts.Value.Kms;

    /// <inheritdoc/>
    public async Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default)
    {
        var current = await store.LoadCurrentAsync(ct) ?? await BootstrapAsync(ct);
        if (current.KmsKeyId is null)
            throw new InvalidOperationException(
                $"signing-key: current kid '{current.Kid}' has no kms_key_id — was it created by FileKeyProvider? " +
                "Clear the signing_keys table or switch back to Mode=file.");
        var sig = await kms.AsymmetricSignAsync(current.KmsKeyId, payload, ct);
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

    /// <inheritdoc/>
    public Task<string> GetKeyForSigningAsync(string kid, CancellationToken ct = default)
    {
        if (!KeyId.IsValid(kid))
            throw new InvalidKidFormatException(kid);

        // For KMS provider the key material is held by KMS; return the KMS resource name.
        return Task.FromResult(BuildKmsKeyId(kid));
    }

    // ── Private helpers ──────────────────────────────────────────────────────

    private async Task<SigningKey> BootstrapAsync(CancellationToken ct)
    {
        var existing = await store.LoadCurrentAsync(ct);
        if (existing is not null)
            return existing;

        var kid = KeyId.Generate(_env, "es256", clock);
        var kmsKeyId = BuildKmsKeyId(kid);

        // Retrieve and cache the public key JWK from KMS
        var pubJwk = await kms.GetPublicKeyJwkAsync(kmsKeyId, kid, ct);

        var row = new SigningKey
        {
            Kid = kid,
            Algorithm = "ES256",
            Status = KeyStatus.Current,
            KmsKeyId = kmsKeyId,
            PublicKeyJwkJson = pubJwk,
            CreatedAt = clock.GetUtcNow().UtcDateTime,
            PromotedAt = clock.GetUtcNow().UtcDateTime,
        };

        try
        {
            await store.InsertAsync(row, ct);
        }
        catch (Microsoft.EntityFrameworkCore.DbUpdateException)
        {
            logger.LogDebug("signing-key: concurrent bootstrap detected for env={Env}, re-reading", _env);
            return await store.LoadCurrentAsync(ct)
                ?? throw new InvalidOperationException("signing-key: concurrent bootstrap succeeded but LoadCurrentAsync returned null");
        }

        logger.LogInformation("signing-key: bootstrapped current KMS kid={Kid} kmsKeyId={KmsKeyId}", kid, kmsKeyId);
        return row;
    }

    /// <summary>
    /// Constructs the fully-qualified KMS resource name for a new signing key version.
    /// Format: <c>projects/{p}/locations/{l}/keyRings/{kr}/cryptoKeys/{kid}/cryptoKeyVersions/1</c>.
    /// </summary>
    private string BuildKmsKeyId(string kid)
        => $"projects/{_kmsOpts.ProjectId}/locations/{_kmsOpts.LocationId}" +
           $"/keyRings/{_kmsOpts.KeyRing}/cryptoKeys/{kid}/cryptoKeyVersions/1";
}
