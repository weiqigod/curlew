using System.Text;

namespace ApiTool.Backend.Audit;

/// <summary>Formats a list of <see cref="AuditLogEntryDto"/> as RFC 4180 CSV.</summary>
public static class AuditLogCsvFormatter
{
    /// <summary>The CSV header row (no line ending).</summary>
    internal const string HeaderRow =
        "created_at,event_type,user_id,user_email,target_type,target_id,success,ip_address";

    /// <summary>Returns RFC 4180 CSV bytes for the given entries.</summary>
    public static byte[] Format(IReadOnlyList<AuditLogEntryDto> entries)
    {
        var sb = new StringBuilder();
        sb.AppendLine(HeaderRow);
        foreach (var e in entries)
            sb.AppendLine(FormatRow(e));
        return Encoding.UTF8.GetBytes(sb.ToString());
    }

    /// <summary>
    /// Formats a single <see cref="AuditLogEntryDto"/> as a CSV row (no line ending).
    /// Used by <see cref="AuditLogExportStreamer"/> for row-at-a-time streaming.
    /// </summary>
    internal static string FormatRow(AuditLogEntryDto e) =>
        string.Join(',', new[]
        {
            Quote(e.CreatedAt.ToString("O")),
            Quote(e.EventType),
            Quote(e.UserId ?? string.Empty),
            Quote(e.UserEmail ?? string.Empty),
            Quote(e.TargetType ?? string.Empty),
            Quote(e.TargetId ?? string.Empty),
            Quote(e.Success ? "true" : "false"),
            Quote(e.IpAddress ?? string.Empty),
        });

    /// <summary>RFC 4180 quoting: wraps in double-quotes if the field contains comma, double-quote
    /// or newline; escapes internal double-quotes by doubling them.
    /// Fields starting with formula-injection characters are prefixed with a single-quote.</summary>
    private static string Quote(string field)
    {
        // Neutralise potential CSV injection (Excel formula injection)
        if (field.Length > 0 && field[0] is '=' or '+' or '-' or '@')
            field = "'" + field;

        if (field.Contains(',') || field.Contains('"') || field.Contains('\n') || field.Contains('\r'))
            return '"' + field.Replace("\"", "\"\"") + '"';

        return field;
    }
}
