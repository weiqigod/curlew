package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeExistingFile creates a markdown file with a sentinel block for a given slug.
func makeExistingFile(t *testing.T, dir, slug, runID, aboveContent, belowContent string) string {
	t.Helper()
	path := filepath.Join(dir, slug+".md")
	content := aboveContent +
		SentinelBeginPrefix + "id=req-1 slug=" + slug + " run=" + runID + SentinelSuffix + "\n" +
		"## Response (deterministic)\n" +
		"OLD_BLOCK\n" +
		SentinelEndPrefix + "id=req-1 slug=" + slug + " run=" + runID + SentinelSuffix + "\n" +
		belowContent
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// TestMarkdown_Splice_MatchRewritesRegion verifies that when the BEGIN/END
// sentinels match the current slug, WriteReport rewrites only the bytes
// between the sentinels, preserving content above and below verbatim.
func TestMarkdown_Splice_MatchRewritesRegion(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()

	above := "# Get user\n\n## Notes\n\nAGENT_ABOVE\n"
	below := "\n## Analysis\n\nAGENT_BELOW\n"
	makeExistingFile(t, dir, "get-user", fixedRunID, above, below)

	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	content := string(got)

	// Above content preserved.
	if !strings.Contains(content, "AGENT_ABOVE") {
		t.Error("AGENT_ABOVE content above sentinel was not preserved")
	}
	// Below content preserved.
	if !strings.Contains(content, "AGENT_BELOW") {
		t.Error("AGENT_BELOW content below sentinel was not preserved")
	}
	// Old block replaced.
	if strings.Contains(content, "OLD_BLOCK") {
		t.Error("OLD_BLOCK should have been replaced by splice")
	}
	// New sentinel content present.
	if !strings.Contains(content, "## Response (deterministic)") {
		t.Error("new sentinel content should be present after splice")
	}
}

// TestMarkdown_Splice_RenameAppends verifies that when the existing slug
// differs from the current slug (request renamed), the formatter appends a
// new sentinel block and emits a stderr warning.
func TestMarkdown_Splice_RenameAppends(t *testing.T) {
	dir := t.TempDir()

	// Create a file named new-slug.md that contains an old-slug sentinel,
	// simulating a request renamed from "old-slug" to "new-slug".
	oldSlugFile := filepath.Join(dir, "new-slug.md")
	oldContent := "# Old file\n" +
		SentinelBeginPrefix + "id=req-1 slug=old-slug run=" + fixedRunID + SentinelSuffix + "\n" +
		"OLD_CONTENT\n" +
		SentinelEndPrefix + "id=req-1 slug=old-slug run=" + fixedRunID + SentinelSuffix + "\n"
	if err := os.WriteFile(oldSlugFile, []byte(oldContent), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	report := makePassReport()
	report.Requests[0].Slug = "new-slug"

	var errBuf strings.Builder
	if err := WriteReport(report, dir, WriteOptions{Stderr: &errBuf}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "new-slug.md"))
	content := string(got)

	// Orphan old block preserved.
	if !strings.Contains(content, "OLD_CONTENT") {
		t.Error("orphan OLD_CONTENT should be preserved")
	}
	// New sentinel block appended.
	if !strings.Contains(content, "slug=new-slug") {
		t.Error("new sentinel with slug=new-slug should be appended")
	}
	// Stderr warning must include "slug changed" and name the orphaned slug.
	warnMsg := errBuf.String()
	if !strings.Contains(warnMsg, "slug changed") {
		t.Errorf("expected slug change warning in stderr, got: %s", warnMsg)
	}
	if !strings.Contains(warnMsg, "old-slug") {
		t.Errorf("expected orphaned slug 'old-slug' named in warning, got: %s", warnMsg)
	}
}

// TestMarkdown_Splice_MalformedWritesDotNew verifies that files with malformed
// sentinels (BEGIN without END, END without BEGIN, or nested BEGIN) cause the
// formatter to write <slug>.md.new alongside the original, leaving it untouched.
func TestMarkdown_Splice_MalformedWritesDotNew(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		warnText string
	}{
		{
			name: "begin_without_end",
			content: "# Get user\n" +
				SentinelBeginPrefix + "id=req-1 slug=get-user run=" + fixedRunID + SentinelSuffix + "\n" +
				"INCOMPLETE\n",
			warnText: "malformed sentinel",
		},
		{
			name: "end_without_begin",
			content: "# Get user\n" +
				"SOME_CONTENT\n" +
				SentinelEndPrefix + "id=req-1 slug=get-user run=" + fixedRunID + SentinelSuffix + "\n",
			warnText: "malformed sentinel",
		},
		{
			name: "nested_begin",
			content: "# Get user\n" +
				SentinelBeginPrefix + "id=req-1 slug=get-user run=" + fixedRunID + SentinelSuffix + "\n" +
				SentinelBeginPrefix + "id=req-2 slug=get-user run=" + fixedRunID + SentinelSuffix + "\n" +
				"NESTED\n" +
				SentinelEndPrefix + "id=req-1 slug=get-user run=" + fixedRunID + SentinelSuffix + "\n",
			warnText: "malformed sentinel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := makePassReport()
			dir := t.TempDir()

			target := filepath.Join(dir, "get-user.md")
			if err := os.WriteFile(target, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write malformed: %v", err)
			}

			var errBuf strings.Builder
			if err := WriteReport(report, dir, WriteOptions{Stderr: &errBuf}); err != nil {
				t.Fatalf("WriteReport: %v", err)
			}

			// Original untouched.
			original, _ := os.ReadFile(target)
			if string(original) != tc.content {
				t.Errorf("original file should be untouched when sentinel is malformed (%s)", tc.name)
			}
			// .new file created.
			if _, err := os.Stat(target + ".new"); os.IsNotExist(err) {
				t.Errorf("expected get-user.md.new to be created (%s)", tc.name)
			}
			// Stderr warning.
			if !strings.Contains(errBuf.String(), tc.warnText) {
				t.Errorf("expected %q in stderr, got: %s (%s)", tc.warnText, errBuf.String(), tc.name)
			}
		})
	}
}

// TestMarkdown_Splice_NoSentinelWritesDotNew verifies that an existing file
// without any sentinels is treated as user-owned: the formatter writes
// <slug>.md.new alongside and emits a stderr warning; the original is untouched.
func TestMarkdown_Splice_NoSentinelWritesDotNew(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()

	// Create user-handwritten file with no sentinels.
	handwritten := "# User handwrote this\n\nSome notes.\n"
	target := filepath.Join(dir, "get-user.md")
	if err := os.WriteFile(target, []byte(handwritten), 0o644); err != nil {
		t.Fatalf("write handwritten: %v", err)
	}

	var errBuf strings.Builder
	if err := WriteReport(report, dir, WriteOptions{Stderr: &errBuf}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	// Original untouched.
	original, _ := os.ReadFile(target)
	if string(original) != handwritten {
		t.Error("original file should be untouched when it has no sentinels")
	}
	// .new file created.
	if _, err := os.Stat(target + ".new"); os.IsNotExist(err) {
		t.Error("expected get-user.md.new to be created")
	}
	// Stderr warning.
	if !strings.Contains(errBuf.String(), "no sentinel pair") {
		t.Errorf("expected 'no sentinel pair' in stderr, got: %s", errBuf.String())
	}
}

// TestMarkdown_Splice_MismatchedRunStillSplices verifies that when the slug
// matches but the run= attribute differs (volatile), the formatter still splices.
func TestMarkdown_Splice_MismatchedRunStillSplices(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()

	// Create existing file with a different run ID (must be exactly 32 hex chars).
	const otherRunID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	above := "# Get user\n\n## Notes\n\nAGENT_NOTE\n"
	below := "\n## Analysis\n"
	makeExistingFile(t, dir, "get-user", otherRunID, above, below)

	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	content := string(got)

	// Agent content preserved.
	if !strings.Contains(content, "AGENT_NOTE") {
		t.Error("AGENT_NOTE should be preserved when run= differs but slug matches")
	}
	// Current run ID appears.
	if !strings.Contains(content, fixedRunID) {
		t.Error("new run ID should appear in the spliced sentinel")
	}
}

// TestMarkdown_Splice_SecondRunAfterOrphanAppend verifies that a second run
// after actionAppendOrphan (request rename) still splices the new sentinel block
// correctly. The file now has two complete sentinel pairs (old-slug + new-slug);
// parseSentinels must find the pair matching the current slug and splice it,
// not treat the second BEGIN as a nested/malformed sentinel.
func TestMarkdown_Splice_SecondRunAfterOrphanAppend(t *testing.T) {
	dir := t.TempDir()

	// Simulate post-orphan-append state: file has two complete sentinel pairs.
	// old-slug pair first, new-slug pair appended below.
	const oldRunID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fileContent := "# Old file\n" +
		SentinelBeginPrefix + "id=req-1 slug=old-slug run=" + oldRunID + SentinelSuffix + "\n" +
		"OLD_CONTENT\n" +
		SentinelEndPrefix + "id=req-1 slug=old-slug run=" + oldRunID + SentinelSuffix + "\n" +
		"\n" +
		SentinelBeginPrefix + "id=req-1 slug=new-slug run=" + fixedRunID + SentinelSuffix + "\n" +
		"FIRST_NEW_CONTENT\n" +
		SentinelEndPrefix + "id=req-1 slug=new-slug run=" + fixedRunID + SentinelSuffix + "\n"

	target := filepath.Join(dir, "new-slug.md")
	if err := os.WriteFile(target, []byte(fileContent), 0o644); err != nil {
		t.Fatalf("write two-pair file: %v", err)
	}

	// Second run with the same new-slug: should splice (not write .md.new or emit malformed warning).
	report := makePassReport()
	report.Requests[0].Slug = "new-slug"

	var errBuf strings.Builder
	if err := WriteReport(report, dir, WriteOptions{Stderr: &errBuf}); err != nil {
		t.Fatalf("WriteReport second run: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	// No .md.new file must be created.
	if _, statErr := os.Stat(target + ".new"); statErr == nil {
		t.Error("new-slug.md.new must NOT be created on second run after orphan-append")
	}
	// No "malformed sentinel" warning.
	if strings.Contains(errBuf.String(), "malformed sentinel") {
		t.Errorf("second run after orphan-append must not warn 'malformed sentinel', got: %s", errBuf.String())
	}
	// Old-slug orphan content is preserved.
	if !strings.Contains(content, "OLD_CONTENT") {
		t.Error("orphan OLD_CONTENT from old-slug pair must be preserved")
	}
	// New sentinel block is present (spliced).
	if !strings.Contains(content, "slug=new-slug") {
		t.Error("spliced sentinel for new-slug must be present")
	}
	// FIRST_NEW_CONTENT was replaced (splice rewrites the new-slug block).
	if strings.Contains(content, "FIRST_NEW_CONTENT") {
		t.Error("FIRST_NEW_CONTENT must be replaced by the splice")
	}
}

// TestMarkdown_Splice_FreshWrite verifies that a completely new file is written
// atomically when no existing file is present.
func TestMarkdown_Splice_FreshWrite(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()

	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	path := filepath.Join(dir, "get-user.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("get-user.md should be created on fresh write")
	}
}
