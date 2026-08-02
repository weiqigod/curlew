using System.Text.Json;
using ApiTool.Backend.Auth;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Http;
using HttpResults = Microsoft.AspNetCore.Http.Results;

namespace ApiTool.Backend.Notifications;

/// <summary>Registers the notification endpoints onto the route builder.</summary>
public static class NotificationsEndpoints
{
    private static readonly JsonSerializerOptions SnakeCaseOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.SnakeCaseLower,
        PropertyNameCaseInsensitive = true,
    };

    /// <summary>Maps all notification endpoints onto the given route builder.</summary>
    /// <param name="app">The route builder to extend.</param>
    /// <returns>The same builder, for chaining.</returns>
    public static IEndpointRouteBuilder MapNotificationsEndpoints(this IEndpointRouteBuilder app)
    {
        var group = app
            .MapGroup("/api/v1/organizations/{orgId}")
            .RequireAuthorization()
            .WithTags("Notifications");

        group.MapPost("/notification-rules", CreateRule)
            .WithName("CreateNotificationRule")
            .Accepts<CreateNotificationRuleRequest>("application/json")
            .Produces<NotificationRuleDto>(StatusCodes.Status201Created)
            .Produces<ErrorResponse>(StatusCodes.Status400BadRequest)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        group.MapGet("/notification-rules", ListRules)
            .WithName("ListNotificationRules")
            .Produces<ListRulesResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        group.MapDelete("/notification-rules/{ruleId}", DeleteRule)
            .WithName("DeleteNotificationRule")
            .Produces(StatusCodes.Status204NoContent)
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
            .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

        group.MapGet("/notification-deliveries", ListDeliveries)
            .WithName("ListNotificationDeliveries")
            .Produces<ListDeliveriesResponse>()
            .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
            .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

        return app;
    }

    private static readonly IResult Unauthorized401 = HttpResults.Json(
        new ErrorResponse("unauthorized", "Authentication is required. Provide a valid Bearer token."),
        statusCode: StatusCodes.Status401Unauthorized);

    private static async Task<IResult> CreateRule(
        string orgId,
        HttpRequest request,
        CurrentUserAccessor users,
        NotificationsService svc,
        CancellationToken ct)
    {
        CreateNotificationRuleRequest? body;
        try
        {
            body = await request.ReadFromJsonAsync<CreateNotificationRuleRequest>(SnakeCaseOptions, ct);
        }
        catch (JsonException)
        {
            return HttpResults.Json(
                new ErrorResponse("invalid_request", "Request body is not valid JSON."),
                statusCode: StatusCodes.Status400BadRequest);
        }

        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (dto, error, message) = await svc.CreateRuleAsync(userId.Value, orgGuid, body, ct);

        return error switch
        {
            NotificationError.None => HttpResults.Json(dto, statusCode: StatusCodes.Status201Created),
            NotificationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", message ?? "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            NotificationError.InvalidChannel => HttpResults.Json(
                new ErrorResponse("invalid_channel", message ?? "Invalid channel."),
                statusCode: StatusCodes.Status400BadRequest),
            NotificationError.InvalidTarget => HttpResults.Json(
                new ErrorResponse("invalid_target", message ?? "Invalid target."),
                statusCode: StatusCodes.Status400BadRequest),
            NotificationError.InvalidEvents => HttpResults.Json(
                new ErrorResponse("invalid_events", message ?? "Invalid events."),
                statusCode: StatusCodes.Status400BadRequest),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListRules(
        string orgId,
        CurrentUserAccessor users,
        NotificationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (rules, error) = await svc.ListRulesAsync(userId.Value, orgGuid, ct);

        return error switch
        {
            NotificationError.None => HttpResults.Ok(new ListRulesResponse(rules)),
            NotificationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> DeleteRule(
        string orgId,
        string ruleId,
        CurrentUserAccessor users,
        NotificationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        if (!NotificationRuleId.TryParse(ruleId, out var ruleGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("not_found", "Notification rule not found."),
                statusCode: StatusCodes.Status404NotFound);
        }

        var error = await svc.DeleteRuleAsync(userId.Value, orgGuid, ruleGuid, ct);

        return error switch
        {
            NotificationError.None => HttpResults.NoContent(),
            NotificationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            NotificationError.NotFound => HttpResults.Json(
                new ErrorResponse("not_found", "Notification rule not found."),
                statusCode: StatusCodes.Status404NotFound),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    private static async Task<IResult> ListDeliveries(
        string orgId,
        int? limit,
        CurrentUserAccessor users,
        NotificationsService svc,
        CancellationToken ct)
    {
        var userId = await users.ResolveAsync(ct);
        if (userId is null)
            return Unauthorized401;

        if (!OrgId.TryParse(orgId, out var orgGuid))
        {
            return HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden);
        }

        var (deliveries, error) = await svc.ListDeliveriesAsync(userId.Value, orgGuid, limit ?? 50, ct);

        return error switch
        {
            NotificationError.None => HttpResults.Ok(new ListDeliveriesResponse(deliveries)),
            NotificationError.PermissionDenied => HttpResults.Json(
                new ErrorResponse("permission_denied", "Permission denied."),
                statusCode: StatusCodes.Status403Forbidden),
            _ => HttpResults.StatusCode(StatusCodes.Status500InternalServerError),
        };
    }

    /// <summary>Response wrapper for list rules.</summary>
    /// <param name="Rules">Ordered list of notification rules for the organization.</param>
    public sealed record ListRulesResponse(IReadOnlyList<NotificationRuleDto> Rules);

    /// <summary>Response wrapper for list deliveries.</summary>
    /// <param name="Deliveries">Newest-first list of delivery attempts.</param>
    public sealed record ListDeliveriesResponse(IReadOnlyList<NotificationDeliveryDto> Deliveries);
}
