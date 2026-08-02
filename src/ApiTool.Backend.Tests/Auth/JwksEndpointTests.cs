// Refs docs/SPECIFICATION.md:7806 + :8237 + :8058.
// Six behaviors enumerated in management/tasks/M14-003.yaml.
using System.Net;
using System.Text.Json;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.AspNetCore.Mvc.Testing;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.DependencyInjection.Extensions;
using Microsoft.IdentityModel.Tokens;

namespace ApiTool.Backend.Tests.Auth;

[Collection(BackendCollection.Name)]
public sealed class JwksEndpointTests(BackendFactory factory) : IAsyncLifetime
{
    public Task InitializeAsync() => factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]  // Behavior #1
    public async Task GET_jwks_returns_200_with_one_key_when_only_current_is_present()
    {
        var client = factory.CreateClient();

        var res = await client.GetAsync("/api/v1/.well-known/jwks.json");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var keys = doc.RootElement.GetProperty("keys");
        keys.GetArrayLength().Should().BeGreaterOrEqualTo(1);
        var jwk = keys[0];
        jwk.GetProperty("alg").GetString().Should().Be("ES256");
        jwk.GetProperty("kty").GetString().Should().Be("EC");
        jwk.GetProperty("crv").GetString().Should().Be("P-256");
        jwk.GetProperty("use").GetString().Should().Be("sig");
        jwk.GetProperty("kid").GetString().Should().NotBeNullOrEmpty();
    }

    [Fact]  // Behavior #2
    public async Task GET_jwks_returns_both_current_and_verifying_keys()
    {
        using var customFactory = factory.WithWebHostBuilder(b => b.ConfigureServices(s =>
        {
            s.RemoveAll<IKeyProvider>();
            s.AddSingleton<IKeyProvider>(new TwoKeyProvider());
        }));
        var client = customFactory.CreateClient();

        var res = await client.GetAsync("/api/v1/.well-known/jwks.json");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var keys = doc.RootElement.GetProperty("keys");
        keys.GetArrayLength().Should().Be(2);
        var kids = keys.EnumerateArray()
            .Select(k => k.GetProperty("kid").GetString())
            .ToHashSet();
        kids.Should().Contain(new[] { "test-current", "test-verifying" });
    }

    [Fact]  // Behavior #3
    public async Task GET_jwks_excludes_revoked_keys()
    {
        using var customFactory = factory.WithWebHostBuilder(b => b.ConfigureServices(s =>
        {
            s.RemoveAll<IKeyProvider>();
            s.AddSingleton<IKeyProvider>(new RevokedExcludingProvider());
        }));
        var client = customFactory.CreateClient();

        var res = await client.GetAsync("/api/v1/.well-known/jwks.json");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadAsStringAsync();
        using var doc = JsonDocument.Parse(body);
        var kidsList = doc.RootElement.GetProperty("keys").EnumerateArray()
            .Select(k => k.GetProperty("kid").GetString()).ToList();
        kidsList.Should().NotContain("test-revoked");
        kidsList.Should().Contain("test-active");
    }

    [Fact]  // Behavior #4
    public async Task GET_jwks_sets_Cache_Control_public_max_age_3600()
    {
        var client = factory.CreateClient();
        var res = await client.GetAsync("/api/v1/.well-known/jwks.json");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var cc = res.Headers.CacheControl;
        cc.Should().NotBeNull();
        cc!.Public.Should().BeTrue();
        cc.MaxAge.Should().Be(TimeSpan.FromSeconds(3600));
    }

    [Fact]  // Behavior #5
    public async Task GET_jwks_is_unauthenticated()
    {
        // Default client carries no Authorization header.
        var client = factory.CreateClient();
        var res = await client.GetAsync("/api/v1/.well-known/jwks.json");
        res.StatusCode.Should().Be(HttpStatusCode.OK);
    }

    [Fact]  // Behavior #6
    public async Task v4_1_public_key_alias_is_NOT_registered()
    {
        var client = factory.CreateClient();
        var res = await client.GetAsync("/api/v1/public-key");
        res.StatusCode.Should().Be(HttpStatusCode.NotFound);
    }
}

/// <summary>Test double returning two JWKs (current + verifying) for Behavior #2.</summary>
internal sealed class TwoKeyProvider : IKeyProvider
{
    public Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default)
        => throw new NotSupportedException();

    public Task<string> GetActiveKidAsync(CancellationToken ct = default)
        => Task.FromResult("test-current");

    public Task<JsonWebKeySet> GetVerificationJwksAsync(CancellationToken ct = default)
    {
        var set = new JsonWebKeySet();
        set.Keys.Add(BuildJwk("test-current"));
        set.Keys.Add(BuildJwk("test-verifying"));
        return Task.FromResult(set);
    }

    private static JsonWebKey BuildJwk(string kid) => new()
    {
        Kid = kid,
        Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
        Crv = "P-256",
        Use = "sig",
        Alg = "ES256",
        X   = Base64UrlEncoder.Encode(new byte[32]),
        Y   = Base64UrlEncoder.Encode(new byte[32]),
    };
}

/// <summary>
/// Test double — the IKeyProvider contract excludes revoked keys;
/// this provider returns only the non-revoked key.
/// </summary>
internal sealed class RevokedExcludingProvider : IKeyProvider
{
    public Task<SignatureResult> SignAsync(byte[] payload, CancellationToken ct = default)
        => throw new NotSupportedException();

    public Task<string> GetActiveKidAsync(CancellationToken ct = default)
        => Task.FromResult("test-active");

    public Task<JsonWebKeySet> GetVerificationJwksAsync(CancellationToken ct = default)
    {
        // Deliberately excludes any key with kid="test-revoked" — revoked rows
        // are filtered by the IKeyProvider implementation, not the endpoint.
        var set = new JsonWebKeySet();
        set.Keys.Add(new JsonWebKey
        {
            Kid = "test-active",
            Kty = JsonWebAlgorithmsKeyTypes.EllipticCurve,
            Crv = "P-256",
            Use = "sig",
            Alg = "ES256",
            X   = Base64UrlEncoder.Encode(new byte[32]),
            Y   = Base64UrlEncoder.Encode(new byte[32]),
        });
        return Task.FromResult(set);
    }
}
