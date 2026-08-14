package openapi

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"gopkg.in/yaml.v3"
)

// §18.8's 3.1-to-3.0 translation table.
//
// Every row is a construct a real 3.1 document contains and a sentence about
// what becomes of it — and two of the four say a warning is issued, which is
// the part that decides whether an import quietly changes a document's meaning
// or tells you it did. §11C.9 was the importer refusing 3.1 documents outright;
// this is what it does now that it accepts them, and nothing had run the rows.

// doc31 wraps a fragment in the smallest document that declares 3.1.
func doc31(extra, schema string) string {
	out := "openapi: 3.1.0\ninfo:\n  title: t\n  version: \"1\"\n"
	out += extra
	out += "paths:\n  /a:\n    get:\n      responses:\n        \"200\":\n" +
		"          description: ok\n          content:\n            application/json:\n" +
		"              schema:\n"
	for _, line := range strings.Split(strings.TrimRight(schema, "\n"), "\n") {
		out += "                " + line + "\n"
	}
	return out
}

// relaxed runs relax31 and returns the rewritten document plus its warnings.
func relaxed(t *testing.T, src string) (map[string]any, []string) {
	t.Helper()

	var warnings []string
	out, err := relax31([]byte(src), func(msg string) { warnings = append(warnings, msg) })
	if err != nil {
		t.Fatalf("relax31: %v", err)
	}
	var root map[string]any
	if unmarshalErr := yaml.Unmarshal(out, &root); unmarshalErr != nil {
		t.Fatalf("re-reading the relaxed document: %v\n%s", unmarshalErr, out)
	}
	return root, warnings
}

// responseSchema digs out the schema the fixture put under /a's 200 response.
func responseSchema(t *testing.T, root map[string]any) map[string]any {
	t.Helper()
	node := any(root)
	for _, step := range []string{"paths", "/a", "get", "responses", "200", "content", "application/json", "schema"} {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("walking to the schema: %q is not a mapping", step)
		}
		node = m[step]
	}
	schema, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("the response schema is not a mapping: %#v", node)
	}
	return schema
}

// relax31Case is how one row of §18.8 is exercised.
type relax31Case struct {
	// match is the distinguishing text of the construct cell.
	match string
	// run applies the construct and checks the row's Treatment cell.
	run func(t *testing.T, treatment string)
}

// A treatment cell that mentions a warning must produce one, and one that does
// not must stay quiet: "dropped" and "dropped with a warning" are different
// promises and the table makes both.
func wantsWarning(treatment string) bool {
	return strings.Contains(strings.ToLower(treatment), "warning")
}

func checkWarnings(t *testing.T, construct, treatment string, warnings []string) {
	t.Helper()
	switch {
	case wantsWarning(treatment) && len(warnings) == 0:
		t.Errorf("§18.8 says %s is %q; the import issued no warning", construct, treatment)
	case !wantsWarning(treatment) && len(warnings) > 0:
		t.Errorf("§18.8 says %s is %q, with no warning; the import issued %v", construct, treatment, warnings)
	}
}

var relax31Cases = []relax31Case{
	{
		match: `"null"`,
		run: func(t *testing.T, treatment string) {
			root, warnings := relaxed(t, doc31("", "type: [\"string\", \"null\"]\n"))
			schema := responseSchema(t, root)
			if got := schema["type"]; got != "string" {
				t.Errorf("§18.8 says a nullable union becomes %q; type is %#v", treatment, got)
			}
			if got := schema["nullable"]; got != true {
				t.Errorf("§18.8 says a nullable union becomes %q; nullable is %#v", treatment, got)
			}
			checkWarnings(t, "a nullable union", treatment, warnings)
		},
	},
	{
		match: `"integer"`,
		run: func(t *testing.T, treatment string) {
			root, warnings := relaxed(t, doc31("", "type: [\"string\", \"integer\"]\n"))
			schema := responseSchema(t, root)
			if _, present := schema["type"]; present {
				t.Errorf("§18.8 says a genuine union is %q; type survived as %#v", treatment, schema["type"])
			}
			checkWarnings(t, "a genuine union", treatment, warnings)
		},
	},
	{
		match: "jsonSchemaDialect",
		run: func(t *testing.T, treatment string) {
			src := "openapi: 3.1.0\njsonSchemaDialect: \"https://json-schema.org/draft/2020-12/schema\"\n" +
				"info:\n  title: t\n  version: \"1\"\n  summary: a summary\n" +
				"  license:\n    name: MIT\n    identifier: MIT\n" +
				"paths:\n  /a:\n    get:\n      responses:\n        \"200\":\n          description: ok\n"
			root, warnings := relaxed(t, src)

			if _, present := root["jsonSchemaDialect"]; present {
				t.Errorf("§18.8 says jsonSchemaDialect is %q; it survived", treatment)
			}
			info, _ := root["info"].(map[string]any)
			if info == nil {
				t.Fatal("the relaxed document lost its info object entirely")
			}
			if _, present := info["summary"]; present {
				t.Errorf("§18.8 says info.summary is %q; it survived", treatment)
			}
			if lic, ok := info["license"].(map[string]any); ok {
				if _, present := lic["identifier"]; present {
					t.Errorf("§18.8 says info.license.identifier is %q; it survived", treatment)
				}
				if lic["name"] != "MIT" {
					t.Errorf("dropping license.identifier also removed license.name: %#v", lic)
				}
			}
			if info["title"] != "t" {
				t.Errorf("dropping the 3.1 info keys also removed info.title: %#v", info)
			}
			checkWarnings(t, "the unread 3.1 fields", treatment, warnings)
		},
	},
	{
		match: "webhooks",
		run: func(t *testing.T, treatment string) {
			src := "openapi: 3.1.0\ninfo:\n  title: t\n  version: \"1\"\n" +
				"webhooks:\n  newPet:\n    post:\n      responses:\n        \"200\":\n          description: ok\n" +
				"paths:\n  /a:\n    get:\n      responses:\n        \"200\":\n          description: ok\n"
			root, warnings := relaxed(t, src)

			if _, present := root["webhooks"]; present {
				t.Errorf("§18.8 says webhooks are %q; they survived", treatment)
			}
			checkWarnings(t, "webhooks", treatment, warnings)
			for _, w := range warnings {
				if strings.Contains(w, "newPet") {
					return
				}
			}
			t.Errorf("the webhooks warning does not name the webhook that was dropped: %v", warnings)
		},
	},
}

func TestDocTables_openAPI31TranslationIsWhatHappens(t *testing.T) {
	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "3.1 construct", "Treatment")
	if err != nil {
		t.Fatalf("3.1 construct table: %v", err)
	}
	constructCol, treatmentCol := docs.Column(hdr, "3.1 construct"), docs.Column(hdr, "Treatment")
	if constructCol < 0 || treatmentCol < 0 {
		t.Fatalf("3.1 construct table lost a column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= treatmentCol {
			continue
		}
		construct, treatment := row[constructCol], row[treatmentCol]

		var matched *relax31Case
		for i := range relax31Cases {
			if strings.Contains(construct, relax31Cases[i].match) {
				matched = &relax31Cases[i]
				break
			}
		}
		if matched == nil {
			t.Errorf("§18.8 lists construct %q, which no case exercises; write one rather than "+
				"leaving the translation unproven", construct)
			continue
		}
		seen++
		t.Run(matched.match, func(t *testing.T) { matched.run(t, treatment) })
	}
	if seen == 0 {
		t.Fatal("no constructs read from §18.8")
	}
}

// "A 3.0 document is passed through untouched" — the sentence under the table,
// and the thing that makes the whole translation safe to run.
func TestDocTables_a30DocumentIsUntouched(t *testing.T) {
	data, err := docs.ReadDoc("CLI_SPECIFICATION.md")
	if err != nil {
		t.Fatalf("reading the specification: %v", err)
	}
	if !strings.Contains(data, "A 3.0 document is passed through untouched") {
		t.Fatal("§18.8 no longer promises that a 3.0 document is untouched")
	}

	src := "openapi: 3.0.3\ninfo:\n  title: t\n  version: \"1\"\n" +
		"paths:\n  /a:\n    get:\n      responses:\n        \"200\":\n          description: ok\n"

	var warnings []string
	out, err := relax31([]byte(src), func(msg string) { warnings = append(warnings, msg) })
	if err != nil {
		t.Fatalf("relax31: %v", err)
	}
	if string(out) != src {
		t.Errorf("a 3.0 document came back rewritten:\n--- in ---\n%s\n--- out ---\n%s", src, out)
	}
	if len(warnings) > 0 {
		t.Errorf("a 3.0 document produced warnings: %v", warnings)
	}
}
