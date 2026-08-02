package markdown

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EnsureReportDir creates the target directory via os.MkdirAll(0755).
// The variadic subpath argument is reserved for M9-004 (data-driven
// per-iteration files) and currently unused in M9-002; passing it concatenates
// the elements onto path before creating.
func EnsureReportDir(path string, subpath ...string) error {
	full := path
	if len(subpath) > 0 {
		parts := make([]string, 0, 1+len(subpath))
		parts = append(parts, path)
		parts = append(parts, subpath...)
		full = filepath.Join(parts...)
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		return fmt.Errorf("create report directory %q: %w", full, err)
	}
	return nil
}

// writeAtomic writes data to path using O_EXCL temp-file + rename.
// Two concurrent invocations on the same path produce one valid file
// (the loser's bytes are discarded by os.Rename's atomic replace).
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpPath := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file %q: %w", tmpPath, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file %q -> %q: %w", tmpPath, path, err)
	}
	return nil
}

// writeFile dispatches the write to the correct splice action:
// fresh write, sentinel splice, append orphan, or .md.new fallback.
func writeFile(path, slug string, content []byte, errW io.Writer) error {
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Fresh write: no existing file.
		return writeAtomic(path, content)
	}
	if err != nil {
		return fmt.Errorf("read existing file %q: %w", path, err)
	}

	action, loc, decideErr := decideAction(existing, slug)
	if decideErr != nil {
		// Malformed sentinel: write .md.new; original untouched.
		_, _ = fmt.Fprintf(errW, "apitest: markdown: malformed sentinel in %s; writing %s.new\n", path, path)
		return writeAtomic(path+".new", content)
	}

	switch action {
	case actionSplice:
		// Rewrite only the bytes between (and including) the sentinel lines.
		spliced := spliceSentinelBlock(existing, loc, content)
		return writeAtomic(path, spliced)

	case actionAppendOrphan:
		// Existing slug differs: append new sentinel block below the existing content.
		_, _ = fmt.Fprintf(errW, "apitest: markdown: slug changed (orphaned slug: %s); preserving orphan block in %s\n", loc.Begin.Slug, path)
		newContent := appendOrphanBlock(existing, content)
		return writeAtomic(path, newContent)

	case actionDotNew:
		// No sentinels: treat file as user-owned; write .md.new alongside.
		_, _ = fmt.Fprintf(errW, "apitest: markdown: no sentinel pair in %s; writing %s.new\n", path, path)
		return writeAtomic(path+".new", content)
	}
	// All spliceAction values are handled above; this line is unreachable.
	panic(fmt.Sprintf("markdown: unhandled spliceAction %d", action))
}

// spliceSentinelBlock replaces the sentinel block (from the BEGIN line up to
// and including the END line) in existing with the sentinel block extracted
// from newContent. Content above the BEGIN line and below the END line is
// preserved verbatim, including their original line endings.
func spliceSentinelBlock(existing []byte, loc *sentinelLocation, newContent []byte) []byte {
	lines := splitLines(existing)

	// Extract the sentinel block from newContent.
	newBlock := extractSentinelBlock(newContent)

	var buf bytes.Buffer
	// Lines before the BEGIN sentinel (0 to BeginLine-1).
	for i := 0; i < loc.BeginLine; i++ {
		buf.Write(lines[i])
	}
	// New sentinel block (already LF-terminated).
	buf.Write(newBlock)
	// Lines after the END sentinel (EndLine+1 onwards).
	for i := loc.EndLine + 1; i < len(lines); i++ {
		buf.Write(lines[i])
	}
	return buf.Bytes()
}

// appendOrphanBlock appends a blank line and the new sentinel block below the
// existing content. The orphan block (existing sentinels) is preserved.
func appendOrphanBlock(existing, newContent []byte) []byte {
	newBlock := extractSentinelBlock(newContent)
	var buf bytes.Buffer
	buf.Write(existing)
	// Ensure there is a blank separator before the appended block.
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	buf.Write(newBlock)
	return buf.Bytes()
}

// extractSentinelBlock returns the bytes from (and including) the first BEGIN
// line to (and including) the first END line in src.
func extractSentinelBlock(src []byte) []byte {
	lines := splitLines(src)
	begin := -1
	end := -1
	for i, line := range lines {
		text := strings.TrimRight(string(line), "\r\n")
		if begin < 0 && strings.HasPrefix(text, SentinelBeginPrefix) {
			begin = i
		} else if begin >= 0 && strings.HasPrefix(text, SentinelEndPrefix) {
			end = i
			break
		}
	}
	if begin < 0 || end < 0 {
		return src // fallback: return full content
	}
	var buf bytes.Buffer
	for i := begin; i <= end; i++ {
		buf.Write(lines[i])
	}
	return buf.Bytes()
}

// splitLines splits b into individual lines preserving the line ending bytes
// (\n or \r\n) as part of each element. The last element may not end with \n.
func splitLines(b []byte) [][]byte {
	var lines [][]byte
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			lines = append(lines, b)
			break
		}
		lines = append(lines, b[:i+1])
		b = b[i+1:]
	}
	return lines
}
