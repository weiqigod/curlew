// Integration tests for /internal/keys/* endpoints.
// Refs docs/SPECIFICATION.md:8048 — internal probe and rotation endpoints.
using System.Net;
using System.Net.Http.Json;
using System.Text.Json;
using ApiTool.Backend.Data;
using ApiTool.Backend.Data.Entities;
using ApiTool.Backend.Licensing.Keys;
using ApiTool.Backend.Tests.TestInfrastructure;
using Microsoft.Extensions.DependencyInjection;

namespace ApiTool.Backend.Tests.Licensing;

[Collection(BackendCollection.Name)]
public sealed class InternalKeysEndpointsTests(BackendFactory factory) : IAsyncLifetime
{
    public Task InitializeAsync() => factory.InitializeAsync();
    public Task DisposeAsync() => Task.CompletedTask;

    [Fact]
    public async Task GET_internal_keys_active_returns_kid_matching_format()
    {
        // Confirms the observable command's expected payload shape.
        var client = factory.CreateClient();

        var res = await client.GetAsync("/internal/keys/active");

        res.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await res.Content.ReadFromJsonAsync<JsonElement>();
        body.GetProperty("kid").GetString().Should().MatchRegex(@"^[a-z0-9-]{1,64}$");
    }

    [Fact]
    public async Task POST_internal_keys_rotate_emergency_returns_new_kid_distinct_from_old()
    {
        var client = factory.CreateClient();

        // Ensure a current key is bootstrapped first.
        var oldRes = await client.GetAsync("/internal/keys/active");
        oldRes.StatusCode.Should().Be(HttpStatusCode.OK);
        var oldKid = (await oldRes.Content.ReadFromJsonAsync<JsonElement>()).GetProperty("kid").GetString();

        // Seed a 'next' key directly into the DB so that rotation promotes it.
        const string nextKid = "test-es256-202605-next99";
        using (var scope = factory.Services.CreateScope())
        {
            var db = scope.ServiceProvider.GetRequiredService<AppDbContext>();
            db.SigningKeys.Add(new SigningKey
            {
                Kid = nextKid,
                Algorithm = "ES256",
                Status = KeyStatus.Next,
                PublicKeyJwkJson = "{\"kty\":\"EC\",\"crv\":\"P-256\",\"use\":\"sig\",\"alg\":\"ES256\",\"kid\":\"" + nextKid + "\",\"x\":\"aaaa\",\"y\":\"bbbb\"}",
                CreatedAt = DateTime.UtcNow,
            });
            await db.SaveChangesAsync();
        }

        var rotated = await client.PostAsJsonAsync("/internal/keys/rotate?emergency=true",
            new { reason = "test" });

        rotated.StatusCode.Should().Be(HttpStatusCode.OK);
        var body = await rotated.Content.ReadFromJsonAsync<JsonElement>();
        var newKid = body.GetProperty("kid").GetString();
        newKid.Should().MatchRegex(@"^[a-z0-9-]{1,64}$");
        newKid.Should().NotBe(oldKid, because: "rotation must promote the seeded next key and yield a different kid");
        newKid.Should().Be(nextKid, because: "the seeded next key should become the new current");
    }

}
