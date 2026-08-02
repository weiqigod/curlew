using ApiTool.Backend.Audit;

namespace ApiTool.Backend.Tests.Audit;

/// <summary>Unit tests for <see cref="AuditLogCsvFormatter"/> RFC 4180 quoting and CSV injection protection.</summary>
public sealed class AuditLogCsvFormatterTests
{
    private static AuditLogEntryDto MakeDto(
        string eventType = "test.event",
        string? userId = null,
        string? userEmail = null,
        string? targetType = null,
        string? targetId = null,
        string? ipAddress = null,
        bool success = true,
        string? failureReason = null) =>
        new(
            EventType: eventType,
            UserId: userId,
            UserEmail: userEmail,
            TargetType: targetType,
            TargetId: targetId,
            CreatedAt: new DateTime(2026, 1, 15, 12, 0, 0, DateTimeKind.Utc),
            IpAddress: ipAddress,
            Success: success,
            FailureReason: failureReason);

    [Fact]
    public void Format_wraps_field_with_comma_in_double_quotes()
    {
        var dto = MakeDto(eventType: "org,event");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        csv.Should().Contain("\"org,event\"");
    }

    [Fact]
    public void Format_doubles_embedded_double_quotes_per_rfc4180()
    {
        var dto = MakeDto(eventType: "say \"hello\"");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        // RFC 4180: double-quote is escaped by doubling it, field is wrapped in quotes
        csv.Should().Contain("\"say \"\"hello\"\"\"");
    }

    [Fact]
    public void Format_prefixes_formula_injection_field_starting_with_equals()
    {
        var dto = MakeDto(eventType: "=CMD(42)");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        // The event_type column should be prefixed with a single-quote
        csv.Should().Contain("'=CMD(42)");
    }

    [Fact]
    public void Format_prefixes_formula_injection_field_starting_with_plus()
    {
        var dto = MakeDto(eventType: "+HYPERLINK(\"evil\")");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        csv.Should().Contain("'+HYPERLINK");
    }

    [Fact]
    public void Format_prefixes_formula_injection_field_starting_with_at()
    {
        var dto = MakeDto(eventType: "@SUM(A1:A10)");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        csv.Should().Contain("'@SUM");
    }

    [Fact]
    public void Format_includes_header_row()
    {
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([]));

        csv.Should().StartWith("created_at,event_type,user_id,user_email,target_type,target_id,success,ip_address");
    }

    [Fact]
    public void Format_wraps_field_containing_newline_in_double_quotes()
    {
        var dto = MakeDto(targetType: "line1\nline2");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        csv.Should().Contain("\"line1\nline2\"");
    }

    [Fact]
    public void Format_plain_field_has_no_quotes()
    {
        var dto = MakeDto(eventType: "member.invited");
        var csv = System.Text.Encoding.UTF8.GetString(AuditLogCsvFormatter.Format([dto]));

        // No double-quotes around a plain field
        csv.Should().Contain("member.invited");
        csv.Should().NotContain("\"member.invited\"");
    }
}
