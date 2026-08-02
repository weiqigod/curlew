using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Auth;

/// <summary>
/// Maps <c>POST /api/v1/auth/reauth</c> — issues a 5-minute single-use <c>drto_</c> token
/// to an authenticated user who proves they still know their current password.
/// The returned token is presented as <c>X-Reauth-Token</c> to guard destructive operations
/// such as <c>POST /api/v1/users/me/deletion-requests</c>. Refs docs/SPECIFICATION.md v4-5.
/// </summary>
public static class ReauthEndpoints
{
    /// <summary>Registers the <c>POST /api/v1/auth/reauth</c> endpoint.</summary>
    public static IEndpointRouteBuilder MapReauthEndpoints(this IEndpointRouteBuilder app)
    {
        app.MapPost("/api/v1/auth/reauth", Issue)
            .RequireAuthorization()
            .DisableAntiforgery()
            .Produces<ReauthResponse>(StatusCodes.Status200OK)
            .Produces(StatusCodes.Status401Unauthorized)
            .WithName("IssueReauthToken")
            .WithTags("Auth")
            .WithSummary("Issue a 5-minute single-use re-auth token for destructive operations.");

        return app;
    }

    private static async Task<IResult> Issue(
        ReauthRequest body,
        CurrentUserAccessor users,
        IDeletionReauthService service,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return HttpResults.Unauthorized();

        var (error, token, expiresAt) = await service.IssueAsync(userId.Value, body.Password, ct);

        return error switch
        {
            ReauthError.None => HttpResults.Ok(new ReauthResponse(token!, expiresAt!.Value)),
            ReauthError.WrongPassword => HttpResults.Problem(
                statusCode: StatusCodes.Status401Unauthorized,
                title: "Wrong password",
                detail: "The supplied password is incorrect.",
                extensions: new Dictionary<string, object?> { ["code"] = "wrong_password" }),
            _ => HttpResults.Unauthorized(),
        };
    }
}

/// <summary>Request body for <c>POST /api/v1/auth/reauth</c>.</summary>
public sealed record ReauthRequest(string Password);

/// <summary>Response body for a successful <c>POST /api/v1/auth/reauth</c>.</summary>
public sealed record ReauthResponse(
    [property: System.Text.Json.Serialization.JsonPropertyName("reauth_token")]
    string ReauthToken,
    [property: System.Text.Json.Serialization.JsonPropertyName("expires_at")]
    DateTime ExpiresAt);
