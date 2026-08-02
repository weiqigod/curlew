// Refs: M16-016 — POST body DTO for creating a GitLab integration.
using System.Text.Json.Serialization;

namespace ApiTool.Backend.GitLab.Installations;

/// <summary>
/// Request body for <c>POST /api/v1/integrations/gitlab</c>.
/// All fields use snake_case wire format via <see cref="JsonPropertyNameAttribute"/>.
/// </summary>
public sealed record CreateGitLabIntegrationRequest(
    [property: JsonPropertyName("project_path")] string ProjectPath,
    [property: JsonPropertyName("access_token")] string AccessToken,
    [property: JsonPropertyName("gitlab_base_url")] string? GitLabBaseUrl,
    [property: JsonPropertyName("gitlab_ca_bundle")] string? GitLabCaBundle);
