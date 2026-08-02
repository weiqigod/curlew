using System.Text.Json.Serialization;
using ApiTool.Backend.Organizations;
using Microsoft.AspNetCore.Mvc;

namespace ApiTool.Backend.Compliance.Gdpr;

/// <summary>Response body for a successful <c>POST /api/v1/users/me/deletion-requests</c>.</summary>
/// <param name="FinalizesAt">ISO-8601 UTC timestamp when the 30-day cooldown elapses.</param>
/// <param name="CancellableUntil">Same as <paramref name="FinalizesAt"/> — window to cancel.</param>
/// <param name="CancelUrl">Web app URL for the user to abort the deletion.</param>
public sealed record UserDeletionRequestDto(
    [property: JsonPropertyName("finalizes_at")]    string FinalizesAt,
    [property: JsonPropertyName("cancellable_until")] string CancellableUntil,
    [property: JsonPropertyName("cancel_url")]      string CancelUrl);

/// <summary>Response body for <c>GET /api/v1/users/me/deletion-requests/status</c>.</summary>
public sealed record UserDeletionStatusDto(
    [property: JsonPropertyName("pending_deletion_at")] string? PendingDeletionAt,
    [property: JsonPropertyName("finalizes_at")]         string? FinalizesAt,
    [property: JsonPropertyName("anonymised_at")]        string? AnonymisedAt);

/// <summary>
/// Typed problem-detail response for HTTP 409 <c>owner_cannot_leave</c>.
/// Extends <see cref="ProblemDetails"/> with the <see cref="BlockingOrgs"/> extension
/// property so the OpenAPI schema surfaces <c>blocking_orgs[]</c> explicitly (v4-7).
/// </summary>
public sealed class OwnerCannotLeaveProblemDetails : ProblemDetails
{
    /// <summary>
    /// Organizations that block account deletion because the requesting user is the
    /// sole Owner and other members remain.
    /// </summary>
    [JsonPropertyName("blocking_orgs")]
    public IReadOnlyList<BlockingOrg> BlockingOrgs { get; init; } = [];

    /// <summary>Machine-readable error code for client-side discrimination.</summary>
    [JsonPropertyName("error_code")]
    public string ErrorCode { get; init; } = "OwnerCannotLeave";
}
