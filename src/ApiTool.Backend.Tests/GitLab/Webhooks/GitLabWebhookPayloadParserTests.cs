// Tests for GitLabWebhookPayloadParser — pipeline hook JSON extraction.
using System.Text.Json;
using ApiTool.Backend.GitLab.Webhooks;

namespace ApiTool.Backend.Tests.GitLab.Webhooks;

/// <summary>
/// Unit tests for <see cref="GitLabWebhookPayloadParser"/>.
/// </summary>
public sealed class GitLabWebhookPayloadParserTests
{
    private static JsonDocument Parse(string json) => JsonDocument.Parse(json);

    [Fact]
    public void ParsePipelineHook_extracts_all_fields()
    {
        const string json = """
        {
          "object_kind": "pipeline",
          "object_attributes": {
            "id": 4242,
            "status": "success",
            "sha": "abc123def4567890abc123def4567890abc12345",
            "ref": "main"
          },
          "project": {
            "id": 31337,
            "path_with_namespace": "test-group/test-project"
          }
        }
        """;
        using var doc = Parse(json);
        var payload = GitLabWebhookPayloadParser.ParsePipelineHook(doc);

        payload.PipelineId.Should().Be(4242L);
        payload.Status.Should().Be("success");
        payload.Sha.Should().Be("abc123def4567890abc123def4567890abc12345");
        payload.ProjectId.Should().Be(31337L);
    }

    [Fact]
    public void ParsePipelineHook_different_status_values_are_preserved()
    {
        foreach (var status in new[] { "running", "pending", "failed", "canceled", "skipped" })
        {
            var json = $$"""
            {
              "object_attributes": { "id": 1, "status": "{{status}}", "sha": "aaa" },
              "project": { "id": 1 }
            }
            """;
            using var doc = Parse(json);
            var payload = GitLabWebhookPayloadParser.ParsePipelineHook(doc);
            payload.Status.Should().Be(status);
        }
    }

    [Fact]
    public void ParsePipelineHook_missing_object_attributes_throws()
    {
        using var doc = Parse("""{"project":{"id":1}}""");
        var act = () => GitLabWebhookPayloadParser.ParsePipelineHook(doc);
        act.Should().Throw<InvalidOperationException>();
    }

    [Fact]
    public void ParsePipelineHook_missing_project_throws()
    {
        using var doc = Parse("""{"object_attributes":{"id":1,"status":"running","sha":"abc"}}""");
        var act = () => GitLabWebhookPayloadParser.ParsePipelineHook(doc);
        act.Should().Throw<InvalidOperationException>();
    }

    [Fact]
    public void ParsePipelineHook_missing_sha_throws()
    {
        using var doc = Parse("""{"object_attributes":{"id":1,"status":"running"},"project":{"id":1}}""");
        var act = () => GitLabWebhookPayloadParser.ParsePipelineHook(doc);
        act.Should().Throw<InvalidOperationException>();
    }
}
