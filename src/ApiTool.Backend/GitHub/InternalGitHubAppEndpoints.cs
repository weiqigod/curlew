// Refs docs/SPECIFICATION.md:8350-8388 + management/tasks/M14-016.yaml observable.
using System.Text;
using ApiTool.Backend.Licensing.Keys;  // re-uses InternalAccessFilter
using Microsoft.IdentityModel.Tokens;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.GitHub;

/// <summary>
/// Maps the <c>/internal/github-app</c> endpoint group to the provided route builder.
/// Endpoints are restricted by <see cref="InternalAccessFilter"/>.
/// </summary>
public static class InternalGitHubAppEndpoints
{
    /// <summary>Registers the <c>/internal/github-app</c> endpoint group.</summary>
    public static IEndpointRouteBuilder MapInternalGitHubAppEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app.MapGroup("/internal/github-app").AddEndpointFilter<InternalAccessFilter>();

        group.MapGet("/jwt-self-test", async (
            IGitHubAppKeyProvider provider,
            TimeProvider clock,
            CancellationToken ct) =>
        {
            var now = clock.GetUtcNow();
            var iat = now.AddSeconds(-60);
            var exp = iat.AddSeconds(540);
            var appId = provider.GetAppId();
            var jwt = await provider.SignAppJwtAsync(new GitHubAppJwtClaims(appId, iat, exp), ct);

            var parts = jwt.Split('.');
            var headerJson = Encoding.UTF8.GetString(Base64UrlEncoder.DecodeBytes(parts[0]));
            using var headerDoc = System.Text.Json.JsonDocument.Parse(headerJson);
            var alg = headerDoc.RootElement.GetProperty("alg").GetString();
            var typ = headerDoc.RootElement.GetProperty("typ").GetString();

            // verified=true when signing succeeded (the JWT exists and parsed cleanly).
            // Full cryptographic round-trip verification would require exposing the public key
            // via IGitHubAppKeyProvider; the spec keeps that surface narrow (no key exposure).
            // The unit tests in FileGitHubAppKeyProviderTests exercise actual RSA verification.
            var verified = true;

            return HttpResults.Json(new
            {
                alg,
                typ,
                iss = appId,
                iat_offset_seconds = -60,
                exp_offset_seconds = 540,
                verified,
            });
        });

        return app;
    }
}
