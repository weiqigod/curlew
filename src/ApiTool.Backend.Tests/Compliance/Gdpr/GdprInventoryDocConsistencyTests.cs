using System.Text.RegularExpressions;
using ApiTool.Backend.Compliance.Gdpr;
using ApiTool.Backend.Data.GdprAttributes;

namespace ApiTool.Backend.Tests.Compliance.Gdpr;

/// <summary>
/// Asserts that every row in <c>docs/security/data-inventory.md</c> matches the
/// disposition recorded by <see cref="GdprAttributeScanner"/> for the corresponding
/// entity, including the header shape and the InExport/InDeletion column text.
/// Marked [Trait("Category", "DocConsistency")] so it can be filtered out of
/// fast inner-loop test runs without removing it from CI.
/// </summary>
[Trait("Category", "DocConsistency")]
public class GdprInventoryDocConsistencyTests
{
    private static readonly string[] ExpectedHeaders =
        ["Table", "Owner", "InExport", "InDeletion", "Retention", "Notes"];

    private static readonly string DocPath = Path.Combine(
        SolutionRoot(), "docs", "security", "data-inventory.md");

    [Fact]
    public void Doc_exists() =>
        File.Exists(DocPath).Should().BeTrue($"expected file to exist at: {DocPath}");

    [Fact]
    public void ComplianceMd_exists()
    {
        var path = Path.Combine(SolutionRoot(), "docs", "COMPLIANCE.md");
        File.Exists(path).Should().BeTrue($"expected docs/COMPLIANCE.md to exist at: {path}");
    }

    [Fact]
    public void ComplianceMd_links_to_data_inventory()
    {
        var path = Path.Combine(SolutionRoot(), "docs", "COMPLIANCE.md");
        File.ReadAllText(path).Should().Contain("security/data-inventory.md",
            "docs/COMPLIANCE.md must cross-link to the GDPR data inventory (M18-003 DoD)");
    }

    [Fact]
    public void Doc_has_fourteen_data_rows()
    {
        var rows = ParseTableRows(File.ReadAllText(DocPath));
        rows.Should().HaveCount(14, "the inventory must enumerate exactly the 14 v4-5 tables (M18-005 adds deletion_reauth_tokens)");
    }

    [Fact]
    public void Doc_has_expected_column_headers()
    {
        var headers = ParseHeaderRow(File.ReadAllText(DocPath));
        headers.Should().Equal(ExpectedHeaders,
            "the data inventory table headers are part of the M18-003 contract — " +
            "renaming or reordering a column requires updating the test and downstream readers");
    }

    [Fact]
    public void Doc_rows_are_alphabetically_ordered_by_table_name()
    {
        var names = ParseTableRows(File.ReadAllText(DocPath))
            .Select(r => r.TableName)
            .ToList();
        names.Should().BeInAscendingOrder(StringComparer.Ordinal,
            "alphabetical order helps reviewers spot missing entries at a glance");
    }

    [Fact]
    public void Doc_row_dispositions_match_scanner_manifest()
    {
        var rows = ParseTableRows(File.ReadAllText(DocPath));
        var manifest = GdprAttributeScanner.Manifest;

        var docMap = rows.ToDictionary(r => r.TableName, r => r.Disposition, StringComparer.Ordinal);
        var codeMap = manifest.Entries.ToDictionary(e => e.TableName, e => e.Disposition, StringComparer.Ordinal);

        var onlyInDoc = docMap.Keys.Except(codeMap.Keys, StringComparer.Ordinal).ToList();
        var onlyInCode = codeMap.Keys.Except(docMap.Keys, StringComparer.Ordinal).ToList();

        onlyInDoc.Should().BeEmpty(
            $"tables in data-inventory.md not found in scanner manifest: {string.Join(", ", onlyInDoc)}");
        onlyInCode.Should().BeEmpty(
            $"tables in scanner manifest not found in data-inventory.md: {string.Join(", ", onlyInCode)}");

        foreach (var (tableName, docDisposition) in docMap)
        {
            var codeDisposition = codeMap[tableName];
            docDisposition.Should().Be(codeDisposition,
                $"table '{tableName}' has disposition {docDisposition} in the doc but {codeDisposition} in code — update data-inventory.md or the entity attributes");
        }
    }

    [Fact]
    public void Doc_InExport_column_matches_disposition()
    {
        // For each row, the InExport column must be "yes" iff the disposition is one of the
        // InExport* dispositions, and "no" otherwise. Catches a doc that silently lies in the
        // InExport column while the Notes column (and disposition-substring scan) still passes.
        var rows = ParseTableRows(File.ReadAllText(DocPath));

        foreach (var row in rows)
        {
            var expectedInExport = IsInExport(row.Disposition) ? "yes" : "no";
            row.InExport.Should().Be(expectedInExport,
                $"table '{row.TableName}' has disposition {row.Disposition}, so InExport column must be '{expectedInExport}', not '{row.InExport}'");
        }
    }

    [Fact]
    public void Doc_InDeletion_column_matches_disposition()
    {
        // The InDeletion column carries free text, but must contain one of the canonical tokens
        // that aligns with the disposition: "hard" (hard-delete), "anonymise" (anonymise creator/columns),
        // or "excluded" (no action on deletion).
        var rows = ParseTableRows(File.ReadAllText(DocPath));

        foreach (var row in rows)
        {
            var expectedToken = ExpectedInDeletionToken(row.Disposition);
            row.InDeletion.Should().Contain(expectedToken,
                $"table '{row.TableName}' has disposition {row.Disposition}, so InDeletion column must contain '{expectedToken}', got '{row.InDeletion}'");
        }
    }

    private static bool IsInExport(GdprDisposition d) =>
        d is GdprDisposition.InExportInDeletionHard
          or GdprDisposition.InExportInDeletionAnonymise;

    private static string ExpectedInDeletionToken(GdprDisposition d) => d switch
    {
        GdprDisposition.InExportInDeletionHard => "hard",
        GdprDisposition.InExportInDeletionAnonymise => "anonymise",
        GdprDisposition.ExcludedFromExportAnonymisedInDeletion => "anonymise",
        GdprDisposition.ExcludedFromBoth => "excluded",
        _ => throw new ArgumentOutOfRangeException(nameof(d), d, "unknown disposition"),
    };

    private static IReadOnlyList<string> ParseHeaderRow(string md)
    {
        // The first markdown table row whose first cell is "Table" (plain, not backtick-wrapped).
        foreach (var raw in md.Split('\n'))
        {
            var line = raw.Trim();
            if (!line.StartsWith('|') || !line.EndsWith('|'))
                continue;
            var cols = SplitMarkdownRow(line);
            if (cols.Count >= 1 && cols[0] == "Table")
                return cols;
        }

        throw new InvalidOperationException(
            $"Could not locate header row beginning with 'Table' in {DocPath}");
    }

    /// <summary>Parses the Markdown table and extracts a row record per data row.</summary>
    private static IReadOnlyList<DocRow> ParseTableRows(string md)
    {
        // Match a row whose first cell is a backtick-wrapped identifier. Tolerant of
        // optional whitespace around the leading pipe (so `|`x`|` and `| `x` |` both parse).
        var rowRegex = new Regex(@"^\|\s*`([^`]+)`\s*\|", RegexOptions.Compiled);
        var results = new List<DocRow>();

        foreach (var raw in md.Split('\n'))
        {
            var line = raw.TrimEnd('\r');
            var m = rowRegex.Match(line);
            if (!m.Success)
                continue;

            var cols = SplitMarkdownRow(line);
            if (cols.Count < 6)
                throw new InvalidOperationException(
                    $"Data row for '{m.Groups[1].Value}' has {cols.Count} columns, expected 6: {line}");

            var tableName = m.Groups[1].Value;
            var owner = cols[1];
            var inExport = cols[2];
            var inDeletion = cols[3];
            var retention = cols[4];
            var notes = cols[5];

            var disposition = ParseDispositionFromNotes(notes)
                ?? throw new InvalidOperationException(
                    $"Row for '{tableName}' has no recognisable disposition substring in Notes: {notes}");

            results.Add(new DocRow(tableName, owner, inExport, inDeletion, retention, notes, disposition));
        }

        return results;
    }

    /// <summary>
    /// Splits a markdown table row into its trimmed cell contents, dropping the leading and
    /// trailing pipes. For the row <c>"| a | b | c |"</c> returns <c>["a", "b", "c"]</c>.
    /// </summary>
    private static IReadOnlyList<string> SplitMarkdownRow(string line)
    {
        var trimmed = line.Trim();
        if (trimmed.StartsWith('|')) trimmed = trimmed[1..];
        if (trimmed.EndsWith('|')) trimmed = trimmed[..^1];
        return trimmed.Split('|').Select(s => s.Trim()).ToList();
    }

    private static GdprDisposition? ParseDispositionFromNotes(string notes)
    {
        // Order matters: check longer names before shorter prefixes.
        if (notes.Contains("ExcludedFromExportAnonymisedInDeletion", StringComparison.Ordinal))
            return GdprDisposition.ExcludedFromExportAnonymisedInDeletion;
        if (notes.Contains("InExportInDeletionAnonymise", StringComparison.Ordinal))
            return GdprDisposition.InExportInDeletionAnonymise;
        if (notes.Contains("InExportInDeletionHard", StringComparison.Ordinal))
            return GdprDisposition.InExportInDeletionHard;
        if (notes.Contains("ExcludedFromBoth", StringComparison.Ordinal))
            return GdprDisposition.ExcludedFromBoth;
        return null;
    }

    private static string SolutionRoot()
    {
        // Walk up from the test assembly output directory until we find ApiTool.Backend.sln.
        var dir = AppContext.BaseDirectory;
        while (dir is not null)
        {
            if (File.Exists(Path.Combine(dir, "ApiTool.Backend.sln")))
                return dir;
            dir = Path.GetDirectoryName(dir);
        }

        throw new InvalidOperationException(
            "Could not locate solution root (ApiTool.Backend.sln) from " + AppContext.BaseDirectory);
    }

    private sealed record DocRow(
        string TableName,
        string Owner,
        string InExport,
        string InDeletion,
        string Retention,
        string Notes,
        GdprDisposition Disposition);
}
