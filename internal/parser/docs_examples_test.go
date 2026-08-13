package parser

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The documents' own examples, executed.
//
// §11C.3: docs/MANUAL.md §7.2 put `websocket:` at the request-item level, as a
// sibling of `request:`, in both of its examples, and a collection copied out of
// the manual was rejected with "must have websocket.steps with at least one
// action". Nothing caught it because no test read the manual.
//
// §11C.11: the same class in the other document. CLI_SPECIFICATION §12.3 gave a
// `wait` step a `timeout_ms` that `wait` ignores, so the documented example
// paused for zero milliseconds. It was found by reading, not by a test — the
// first version of this file covered MANUAL.md only, and the specification's
// snippets were executed by nothing.
//
// Both documents are now checked. That alone would not have caught §11C.11,
// because `timeout_ms` parsed perfectly well; the parser had to start rejecting
// a field its action ignores (see validateStepFields) before an example could
// be wrong in a way a parse test can see.
//
// The checks are deliberately structural rather than copies of the examples
// into Go string literals. A copy pins the parser against a snapshot and goes
// stale the moment a document is edited; this fails when a document and the
// parser disagree, which is the actual defect.

var yamlBlockRe = regexp.MustCompile("(?s)```yaml\n(.*?)```")

// docsWithExamples are the documents whose YAML examples must be runnable as
// written. Both describe the same binary, and each has now been wrong about it.
var docsWithExamples = []string{"MANUAL.md", "CLI_SPECIFICATION.md"}

func docExamples(t *testing.T, doc string) map[int]string {
	t.Helper()
	path := filepath.Join("..", "..", "docs", doc)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := string(data)

	examples := map[int]string{}
	for _, m := range yamlBlockRe.FindAllStringSubmatchIndex(text, -1) {
		block := text[m[2]:m[3]]
		trimmed := strings.TrimLeft(block, "\n")

		// Reference blocks that spell out the shape rather than showing a
		// usable collection: appendix C's type signatures (`name: string`,
		// `requests: [RequestItem]`) and prose examples that elide the request
		// with an ellipsis. Neither is meant to be copied and run.
		if isTypeSignature(trimmed) {
			continue
		}
		// Examples that point at files on disk — a PDF fixture, an external
		// query, an included collection. Parsing resolves those, so they cannot
		// be checked outside the project they belong to. This is about the
		// missing fixture, not the YAML.
		if referencesExternalFile(trimmed) {
			continue
		}

		collection, ok := asCollection(trimmed)
		if !ok {
			continue
		}
		line := strings.Count(text[:m[2]], "\n") + 1
		examples[line] = collection
	}
	return examples
}

// asCollection turns a documentation snippet into a parseable collection, and
// reports whether the snippet is one of the shapes worth checking.
//
// The two documents are written differently, and that difference is why
// §11C.11 survived. MANUAL.md shows whole collections; CLI_SPECIFICATION.md
// shows FRAGMENTS — a bare `request:` mapping, a bare `assertions:` mapping —
// because it is a reference rather than a tutorial. A checker that only
// accepted complete collections found nothing at all in the specification, so
// the document where a `wait` step was given a field it ignores was checked by
// nothing.
//
// Wrapping is the whole point: a fragment the reader is expected to paste under
// a request must be valid there.
func asCollection(trimmed string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmed, "name:"):
		// A whole collection — but `name:` also starts environment and
		// curlew.yaml examples, which are not collections.
		if !strings.Contains(trimmed, "requests:") {
			return "", false
		}
		return trimmed, true

	case strings.HasPrefix(trimmed, "requests:"),
		strings.HasPrefix(trimmed, "setup:"),
		strings.HasPrefix(trimmed, "teardown:"):
		return "name: doc example\n" + trimmed, true

	case strings.HasPrefix(trimmed, "request:"):
		// A request mapping, as CLI_SPECIFICATION §12.2 and §12.3 show it.
		return "name: doc example\nrequests:\n  - name: doc request\n" + indent(trimmed, 4), true

	case strings.HasPrefix(trimmed, "assertions:"):
		// An assertions mapping. It needs a request to hang from; the URL is
		// never dialled, because only parsing is under test.
		return "name: doc example\nrequests:\n  - name: doc request\n" +
			"    request:\n      method: GET\n      url: \"https://example.test/\"\n" +
			indent(trimmed, 4), true
	}
	return "", false
}

// indent shifts every non-empty line right by n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

// typeSignatureRe matches a value that names a TYPE rather than giving one —
// `max_duration_ms: int`, `schema: string`. Both documents carry reference
// blocks in that style, and they are not meant to be copied and run.
var typeSignatureRe = regexp.MustCompile(`(?m):\s*(int|string|bool|number|float)\s*(#.*)?$`)

// isTypeSignature reports whether a block describes the shape of a collection
// rather than showing one. These are excluded because failing them would say
// nothing about the binary — but they are excluded narrowly, since an
// over-broad filter is how a documentation test goes quietly vacuous.
func isTypeSignature(block string) bool {
	return strings.Contains(block, "RequestItem") ||
		strings.Contains(block, "...") ||
		strings.Contains(block, " | ") ||
		typeSignatureRe.MatchString(block)
}

// externalFileKeys are the collection fields the parser resolves against the
// filesystem at parse time.
var externalFileKeys = []string{
	"body_file:", "body_binary_file:", "message_template:",
	"query_file:", "fragments:", "include:", "path:", "data:", "schema_file:", "schema:",
}

func referencesExternalFile(block string) bool {
	for _, k := range externalFileKeys {
		if strings.Contains(block, k) {
			return true
		}
	}
	return false
}

func writeExample(t *testing.T, dir, prefix string, line int, block string) string {
	t.Helper()
	file := filepath.Join(dir, prefix+"-"+itoa(line)+".yaml")
	if err := os.WriteFile(file, []byte(block), 0o600); err != nil {
		t.Fatalf("writing example: %v", err)
	}
	return file
}

// TestDocs_collectionExamplesParse requires every complete collection example
// in every document to parse. A reader who copies one out must get a working
// collection, not an error.
func TestDocs_collectionExamplesParse(t *testing.T) {
	for _, doc := range docsWithExamples {
		t.Run(doc, func(t *testing.T) {
			examples := docExamples(t, doc)
			if len(examples) == 0 {
				// A test that finds nothing to check and reports success is
				// worse than no test: it would go on passing after the document
				// was restructured.
				t.Fatalf("no complete collection examples found in docs/%s", doc)
			}
			t.Logf("checking %d complete collection example(s)", len(examples))

			dir := t.TempDir()
			for line, block := range examples {
				file := writeExample(t, dir, "ex", line, block)
				if _, err := ParseFile(file); err != nil {
					t.Errorf("docs/%s:%d — example does not parse: %v", doc, line, err)
				}
			}
		})
	}
}

// TestDocs_websocketExamplesAreComplete guards the shape §11C.3 got wrong: an
// example that parses but declares no steps would satisfy the test above while
// still being uncopyable.
func TestDocs_websocketExamplesAreComplete(t *testing.T) {
	found := 0
	for _, doc := range docsWithExamples {
		examples := docExamples(t, doc)
		dir := t.TempDir()
		for line, block := range examples {
			if !strings.Contains(block, "protocol: websocket") {
				continue
			}
			found++
			col, err := ParseFile(writeExample(t, dir, "ws", line, block))
			if err != nil {
				t.Errorf("docs/%s:%d — websocket example does not parse: %v", doc, line, err)
				continue
			}
			for _, item := range col.Requests.Items {
				if item.Request.Protocol != "websocket" {
					continue
				}
				if item.Request.WebSocket == nil {
					t.Errorf("docs/%s:%d — request %q has no websocket config; "+
						"`websocket:` belongs INSIDE `request:` (§12.3)", doc, line, item.Name)
					continue
				}
				if len(item.Request.WebSocket.Steps) == 0 {
					t.Errorf("docs/%s:%d — request %q declares no websocket steps", doc, line, item.Name)
				}
			}
		}
	}
	if found == 0 {
		t.Fatal("no websocket examples found in any document")
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
