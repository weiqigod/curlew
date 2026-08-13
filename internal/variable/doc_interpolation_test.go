package variable

import (
	"regexp"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §6.1 lists the interpolation forms the engine recognises, and — more usefully
// — the prose beneath it lists two that must *not* match. A table of forms is
// only half a contract: the half that says what is left alone is what stops a
// literal `{user_id}` in a body being eaten.
//
// §6.6 groups the dynamic functions by category. It is a second list of the
// same names the manual's twelve tables carry, in a different document, and
// nothing had ever compared either to the registry.

// docForm matches the `{{...}}` in a Form cell.
var docForm = regexp.MustCompile(`\{\{[^}]+\}\}`)

// specFunc matches a `$name` in the specification's category table.
var specFunc = regexp.MustCompile(`\$([A-Za-z][A-Za-z0-9.]*)`)

// Each documented form must be recognised by one of the engine's patterns.
//
// Checking through a Scope alone was the wrong instrument: the four forms are
// resolved by four different namespaces — plain, fallback, dynamic function,
// vault alias — and a scope with no registry or vault leaves three of them
// untouched, which looks identical to "the engine does not know this form".
func TestDocTables_everyInterpolationFormIsRecognised(t *testing.T) {
	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Form", "Meaning")
	if err != nil {
		t.Fatalf("interpolation form table: %v", err)
	}
	formCol := docs.Column(hdr, "Form")
	if formCol < 0 {
		t.Fatalf("form table lost its Form column: %v", hdr)
	}

	// The namespaces, each with the pattern that owns it.
	namespaces := []struct {
		name string
		re   *regexp.Regexp
	}{
		{"plain reference", varPattern},
		{"default fallback", defaultPattern},
		{"dynamic function", dynPattern},
		{"vault alias", secretsPattern},
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= formCol {
			continue
		}
		form := docForm.FindString(strings.ReplaceAll(row[formCol], `\|`, "|"))
		if form == "" {
			continue
		}
		checked++

		matched := ""
		for _, ns := range namespaces {
			if ns.re.FindString(form) == form {
				matched = ns.name
				break
			}
		}
		if matched == "" {
			t.Errorf("§6.1 documents the form %s, which no interpolation pattern matches — "+
				"it would reach the server as literal text", form)
		}
	}
	if checked == 0 {
		t.Fatal("no forms read from §6.1; the table moved or its header changed")
	}
}

// And the fallback form, which the table is the only place to describe, does
// what its Meaning column says.
func TestDocTables_theFallbackFormSubstitutes(t *testing.T) {
	scope := NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("resolve scope: %v", err)
	}
	got, err := scope.Interpolate("{{user_id|default:123}}")
	if err != nil {
		t.Fatalf("the form §6.1 documents does not interpolate: %v", err)
	}
	if got != "123" {
		t.Errorf("§6.1 calls it a fallback if undefined; it produced %q, want %q", got, "123")
	}
}

// The prose under §6.1 says what must not be interpolated. That is the claim a
// user relies on when a body legitimately contains braces.
func TestDocTables_theFormsThatMustNotMatchAreLeftAlone(t *testing.T) {
	spec, err := docs.ReadDoc("CLI_SPECIFICATION.md")
	if err != nil {
		t.Fatalf("reading the specification: %v", err)
	}
	if !strings.Contains(spec, "does **not** match") {
		t.Fatal("§6.1 no longer states which forms are left alone; this check has nothing to run")
	}

	scope := NewScope(map[string]string{"user_id": "42"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("resolve scope: %v", err)
	}

	// Both forms the specification names, quoted from it.
	for _, literal := range []string{"{user_id}", "{{ user_id }}"} {
		if !strings.Contains(spec, literal) {
			t.Errorf("§6.1 no longer names %q among the forms that do not match", literal)
			continue
		}
		got, interpErr := scope.Interpolate(literal)
		if interpErr != nil {
			t.Errorf("interpolating %q returned an error rather than leaving it alone: %v", literal, interpErr)
			continue
		}
		if got != literal {
			t.Errorf("§6.1 says %q does not match, but it interpolated to %q", literal, got)
		}
	}
}

func TestDocTables_specFunctionCategoriesMatchTheRegistry(t *testing.T) {
	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Category", "Functions")
	if err != nil {
		t.Fatalf("function category table: %v", err)
	}
	col := docs.Column(hdr, "Functions")
	if col < 0 {
		t.Fatalf("category table lost its Functions column: %v", hdr)
	}

	seed := int64(1)
	available := map[string]bool{}
	for _, name := range NewRegistry(&seed).Available() {
		available[name] = true
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		for _, m := range specFunc.FindAllStringSubmatch(row[col], -1) {
			name := strings.TrimSuffix(m[1], ".")
			// The Faker row names the family rather than a function.
			if name == "faker" {
				continue
			}
			checked++
			if !available[name] {
				t.Errorf("§6.6 lists `$%s`, which the registry does not provide", name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no functions read from §6.6; the table moved or its header changed")
	}
}
