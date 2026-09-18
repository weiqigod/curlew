package markdown

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maskVolatileLines replaces the content of volatile timing lines with a fixed
// placeholder so two runs can be compared byte-for-byte.
// Volatile lines are identified by these prefixes:
//   - "duration_ms: "
//   - "wave_index: "
//   - "started_at: "
func maskVolatileLines(b []byte) []byte {
	lines := splitLines(b)
	var buf bytes.Buffer
	for _, line := range lines {
		text := string(line)
		stripped := strings.TrimRight(text, "\r\n")
		if strings.HasPrefix(stripped, "duration_ms: ") ||
			strings.HasPrefix(stripped, "wave_index: ") ||
			strings.HasPrefix(stripped, "started_at: ") {
			// Replace volatile value with a fixed placeholder.
			prefix := stripped[:strings.Index(stripped, ": ")+2]
			buf.WriteString(prefix)
			buf.WriteString("<masked>\n")
		} else {
			buf.Write(line)
		}
	}
	return buf.Bytes()
}

// TestMarkdown_DeterminismRegression verifies that two WriteReport calls with
// identical Report values produce byte-identical sentinel regions after
// masking the three volatile lines (duration_ms, wave_index, started_at).
func TestMarkdown_DeterminismRegression(t *testing.T) {
	report := makePassReport()

	dir1 := t.TempDir()
	if err := WriteReport(report, dir1, WriteOptions{}); err != nil {
		t.Fatalf("first WriteReport: %v", err)
	}
	dir2 := t.TempDir()
	if err := WriteReport(report, dir2, WriteOptions{}); err != nil {
		t.Fatalf("second WriteReport: %v", err)
	}

	got1, _ := os.ReadFile(filepath.Join(dir1, "get-user.md"))
	got2, _ := os.ReadFile(filepath.Join(dir2, "get-user.md"))

	masked1 := maskVolatileLines(got1)
	masked2 := maskVolatileLines(got2)

	if !bytes.Equal(masked1, masked2) {
		t.Errorf("two identical runs differ after masking volatile lines:\n--- run1 ---\n%s\n--- run2 ---\n%s",
			masked1, masked2)
	}
}
