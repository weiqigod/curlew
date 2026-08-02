// Production wrapper for Google.Cloud.Kms.V1.KeyManagementServiceClient.
// This class is only instantiated when ApiTool:KeyProvider:Mode=kms.
// It does NOT pull in the Google.Cloud.Kms.V1 package as a required compile-time
// dependency — the package is optional and behind the keyMode == "kms" branch.
// For the current M14-001 slice we provide a thin HTTP-based implementation
// so the project compiles without the Google KMS SDK; a proper gRPC implementation
// is deferred to the M14 KMS hardening slice.
using System.Net.Http.Json;
using System.Text.Json;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Licensing.Keys;

/// <summary>
/// Production <see cref="IKmsClient"/> implementation that calls the Google Cloud KMS REST API.
/// Only used when <c>ApiTool:KeyProvider:Mode=kms</c>.
/// </summary>
/// <remarks>
/// This is a thin REST adapter. A full gRPC implementation using
/// <c>Google.Cloud.Kms.V1.KeyManagementServiceClient</c> is deferred to a later slice.
/// </remarks>
public sealed class GoogleKmsClient(IHttpClientFactory httpFactory) : IKmsClient
{
    private const string BaseUrl = "https://cloudkms.googleapis.com/v1";

    /// <inheritdoc/>
    public async Task<byte[]> AsymmetricSignAsync(string kmsKeyId, byte[] payload, CancellationToken ct = default)
    {
        var client = httpFactory.CreateClient("kms");
        var hash = System.Security.Cryptography.SHA256.HashData(payload);

        var body = new
        {
            digest = new { sha256 = Convert.ToBase64String(hash) },
        };

        var res = await client.PostAsJsonAsync($"{BaseUrl}/{kmsKeyId}:asymmetricSign", body, ct);
        res.EnsureSuccessStatusCode();

        var json = await res.Content.ReadFromJsonAsync<JsonElement>(ct);
        var sigBase64 = json.GetProperty("signature").GetString()
            ?? throw new InvalidOperationException("KMS AsymmetricSign response missing 'signature' field");

        return Convert.FromBase64String(sigBase64);
    }

    /// <inheritdoc/>
    public async Task<byte[]> AsymmetricSignRsaPkcs1Sha256Async(string kmsKeyId, byte[] payload, CancellationToken ct = default)
    {
        // KMS API is the same endpoint; the key resource type determines the algorithm/padding.
        // For RSA_SIGN_PKCS1_2048_SHA256 the response 'signature' field is the raw signature bytes.
        var client = httpFactory.CreateClient("kms");
        var hash = System.Security.Cryptography.SHA256.HashData(payload);
        var body = new { digest = new { sha256 = Convert.ToBase64String(hash) } };
        var res = await client.PostAsJsonAsync($"{BaseUrl}/{kmsKeyId}:asymmetricSign", body, ct);
        res.EnsureSuccessStatusCode();
        var json = await res.Content.ReadFromJsonAsync<JsonElement>(ct);
        var sigBase64 = json.GetProperty("signature").GetString()
            ?? throw new InvalidOperationException("KMS AsymmetricSign response missing 'signature' field");
        return Convert.FromBase64String(sigBase64);
    }

    /// <inheritdoc/>
    public async Task<string> GetPublicKeyJwkAsync(string kmsKeyId, string kid, CancellationToken ct = default)
    {
        var client = httpFactory.CreateClient("kms");
        var res = await client.GetAsync($"{BaseUrl}/{kmsKeyId}/publicKey", ct);
        res.EnsureSuccessStatusCode();

        var json = await res.Content.ReadFromJsonAsync<JsonElement>(ct);
        var pem = json.GetProperty("pem").GetString()
            ?? throw new InvalidOperationException("KMS GetPublicKey response missing 'pem' field");

        // Import PEM, convert to JWK
        using var ecdsa = System.Security.Cryptography.ECDsa.Create();
        ecdsa.ImportFromPem(pem);
        var p = ecdsa.ExportParameters(includePrivateParameters: false);

        var jwk = new JsonWebKey
        {
            Kid = kid,
            Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
            Crv = "P-256",
            Use = "sig",
            Alg = "ES256",
            X = Base64UrlEncoder.Encode(p.Q.X!),
            Y = Base64UrlEncoder.Encode(p.Q.Y!),
        };
        return JsonSerializer.Serialize(jwk);
    }

    /// <inheritdoc/>
    public async Task<byte[]> EncryptAsync(string kmsKeyId, byte[] plaintext, CancellationToken ct = default)
    {
        var client = httpFactory.CreateClient("kms");
        var body = new { plaintext = Convert.ToBase64String(plaintext) };
        var res = await client.PostAsJsonAsync($"{BaseUrl}/{kmsKeyId}:encrypt", body, ct);
        res.EnsureSuccessStatusCode();
        var json = await res.Content.ReadFromJsonAsync<JsonElement>(ct);
        var ciphertextBase64 = json.GetProperty("ciphertext").GetString()
            ?? throw new InvalidOperationException("KMS Encrypt response missing 'ciphertext' field");
        return Convert.FromBase64String(ciphertextBase64);
    }

    /// <inheritdoc/>
    public async Task<byte[]> DecryptAsync(string kmsKeyId, byte[] ciphertext, CancellationToken ct = default)
    {
        var client = httpFactory.CreateClient("kms");
        var body = new { ciphertext = Convert.ToBase64String(ciphertext) };
        var res = await client.PostAsJsonAsync($"{BaseUrl}/{kmsKeyId}:decrypt", body, ct);
        res.EnsureSuccessStatusCode();
        var json = await res.Content.ReadFromJsonAsync<JsonElement>(ct);
        var plaintextBase64 = json.GetProperty("plaintext").GetString()
            ?? throw new InvalidOperationException("KMS Decrypt response missing 'plaintext' field");
        return Convert.FromBase64String(plaintextBase64);
    }
}
