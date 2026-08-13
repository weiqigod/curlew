package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// §11C.3. docs/MANUAL.md §7.2 put `websocket:` at the request-item level, as a
// sibling of `request:`, in both of its examples. The parser wants it INSIDE
// `request:`, as docs/CLI_SPECIFICATION.md §12.3 correctly shows, so a
// collection copied out of the manual was rejected with
//
//	must have websocket.steps with at least one action
//
// Nothing caught it because no test read the manual. This one does: every
// complete collection example in MANUAL.md — a fenced yaml block beginning at
// column zero with `name:` or `requests:` — must parse.
//
// It is deliberately structural rather than a copy of the examples into Go
// string literals. A copy pins the parser against a snapshot and goes stale the
// moment the manual is edited; this fails when the manual and the parser
// disagree, which is the actual defect.

var yamlBlockRe = regexp.MustCompile("(?s)```yaml\n(.*?)```")

func manualExamples(t *testing.T) map[int]string {
	t.Helper()
	path := filepath.Join("..", "..", "docs", "MANUAL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := string(data)

	examples := map[int]string{}
	for _, m := range yamlBlockRe.FindAllStringSubmatchIndex(text, -1) {
		block := text[m[2]:m[3]]
		trimmed := strings.TrimLeft(block, "\n")
		// Only complete collections. The manual also shows fragments — an
		// indented `websocket:` stanza, a single `assertions:` map — which are
		// illustrative and cannot stand alone.
		if !strings.HasPrefix(trimmed, "name:") && !strings.HasPrefix(trimmed, "requests:") {
			continue
		}
		// Config-file examples (curlew.yaml, environments) share the fence but
		// are not collections.
		if strings.HasPrefix(trimmed, "name:") && !strings.Contains(trimmed, "requests:") {
			continue
		}
		// Reference blocks that spell out the shape rather than showing a
		// usable collection: appendix C's type signatures (`name: string`,
		// `requests: [RequestItem]`) and prose examples that elide the request
		// with an ellipsis. Neither is meant to be copied and run.
		if strings.Contains(trimmed, "RequestItem") || strings.Contains(trimmed, "name: ...") {
			continue
		}
		// A collection example a reader could actually run names a URL.
		if !strings.Contains(trimmed, "url:") {
			continue
		}
		// Examples that point at files on disk — a PDF fixture, an external
		// query, an included collection. Parsing resolves those, so they
		// cannot be checked outside the project they belong to. This is about
		// the missing fixture, not the YAML.
		if referencesExternalFile(trimmed) {
			continue
		}
		line := strings.Count(text[:m[2]], "\n") + 1
		examples[line] = block
	}
	return examples
}

// externalFileKeys are the collection fields the parser resolves against the
// filesystem at parse time.
var externalFileKeys = []string{
	"body_file:", "body_binary_file:", "message_template:",
	"query_file:", "fragments:", "include:", "path:", "data:", "schema_file:",
}

func referencesExternalFile(block string) bool {
	for _, k := range externalFileKeys {
		if strings.Contains(block, k) {
			return true
		}
	}
	return false
}

func TestManual_collectionExamplesParse(t *testing.T) {
	examples := manualExamples(t)
	if len(examples) == 0 {
		// A test that finds nothing to check and reports success is worse than
		// no test: it would go on passing after the manual was restructured.
		t.Fatal("no complete collection examples found in docs/MANUAL.md")
	}

	t.Logf("checking %d complete collection example(s) from docs/MANUAL.md", len(examples))
	dir := t.TempDir()
	for line, block := range examples {
		body := block
		// A collection needs a name; most examples show only `requests:`
		// because the surrounding prose supplies the rest.
		if !strings.HasPrefix(strings.TrimLeft(body, "\n"), "name:") {
			body = "name: manual example\n" + body
		}
		file := filepath.Join(dir, "manual-line-"+itoa(line)+".yaml")
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatalf("writing example: %v", err)
		}
		if _, err := ParseFile(file); err != nil {
			t.Errorf("docs/MANUAL.md:%d — example does not parse: %v\n---\n%s---", line, err, body)
		}
	}
}

// TestManual_websocketExamplesAreComplete guards the specific shape §11C.3 got
// wrong: a websocket example that parses but declares no steps would satisfy
// the test above while still being uncopyable.
func TestManual_websocketExamplesAreComplete(t *testing.T) {
	examples := manualExamples(t)
	dir := t.TempDir()
	found := 0

	for line, block := range examples {
		if !strings.Contains(block, "protocol: websocket") {
			continue
		}
		found++
		body := block
		if !strings.HasPrefix(strings.TrimLeft(body, "\n"), "name:") {
			body = "name: manual example\n" + body
		}
		file := filepath.Join(dir, "ws-line-"+itoa(line)+".yaml")
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatalf("writing example: %v", err)
		}
		col, err := ParseFile(file)
		if err != nil {
			t.Errorf("docs/MANUAL.md:%d — websocket example does not parse: %v", line, err)
			continue
		}
		for _, item := range col.Requests.Items {
			if item.Request.Protocol != "websocket" {
				continue
			}
			if item.Request.WebSocket == nil {
				t.Errorf("docs/MANUAL.md:%d — request %q has no websocket config; "+
					"`websocket:` belongs INSIDE `request:` (§12.3)", line, item.Name)
				continue
			}
			if len(item.Request.WebSocket.Steps) == 0 {
				t.Errorf("docs/MANUAL.md:%d — request %q declares no websocket steps", line, item.Name)
			}
		}
	}

	if found == 0 {
		t.Fatal("no websocket examples found in docs/MANUAL.md")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
