package main

import (
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/output"
)

// TestSkill_outputFormatsTableMatchesBinary asserts output-formats.md's
// "Choosing a format" table lists exactly the formats output.SupportedFormats
// accepts, in both directions — the same shape of check
// TestDocTables_formatTablesMatchTheSupportedSet (doc_name_tables_test.go)
// already runs for docs/CLI_SPECIFICATION.md and docs/MANUAL.md.
//
// It exists because output-formats.md's HTML row is the one edit this task
// makes that would otherwise be held to nothing but the word-anchored
// licensing scan (skill_hygiene_test.go) — "an unguarded doc claim about a
// removed feature" is the exact defect shape this task fixes, and the same
// drift can recur on this table without anyone naming a tier.
func TestSkill_outputFormatsTableMatchesBinary(t *testing.T) {
	tree := scaffoldTreeWithSkill(t, "agent")
	body, ok := tree[".claude/skills/curlew/output-formats.md"]
	if !ok {
		t.Fatal("scaffolded skill missing output-formats.md")
	}

	header, rows := tableUnder(t, body, "Choosing a format")
	col := -1
	for i, h := range header {
		if h == "Format" {
			col = i
			break
		}
	}
	if col < 0 {
		t.Fatalf("output-formats.md: no %q column in header %v", "Format", header)
	}

	documented := map[string]bool{}
	for _, row := range rows {
		if col >= len(row) {
			continue
		}
		if name := docs.FirstName(row[col]); name != "" {
			documented[name] = true
		}
	}
	if len(documented) == 0 {
		t.Fatal("output-formats.md: no formats read from the table — the parser is broken, not the skill")
	}

	supported := map[string]bool{}
	for _, f := range output.SupportedFormats {
		supported[f] = true
	}

	for name := range documented {
		if !supported[name] {
			t.Errorf("output-formats.md documents format %q, which output.SupportedFormats does not list", name)
		}
	}
	for _, f := range output.SupportedFormats {
		if !documented[f] {
			t.Errorf("output-formats.md omits format %q, which the binary accepts", f)
		}
	}
}
