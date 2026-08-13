package variable

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The locale table and the date-layout table.
//
// The locale table is a list of codes a reader will pass to --locale, checked
// in both directions: a documented locale the binary rejects is a run that
// fails on the manual's own advice, and a supported locale with no row is a
// language nobody knows they can ask for.
//
// The layout table is arithmetic of a different kind — every row is a Go time
// layout and the exact string it renders — so it is simply run.

func TestDocTables_everyDocumentedLocaleIsSupported(t *testing.T) {
	hdr, rows, err := docs.TableUnder("MANUAL.md", "locales", "Locale", "Language / Region",
		"Name format", "Phone format")
	if err != nil {
		t.Fatalf("locale table: %v", err)
	}
	col := docs.Column(hdr, "Locale")
	if col < 0 {
		t.Fatalf("locale table lost its Locale column: %v", hdr)
	}

	documented := map[string]bool{}
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		code := docs.FirstName(row[col])
		if code == "" {
			continue
		}
		documented[code] = true
		if err := ValidateLocale(code); err != nil {
			t.Errorf("the manual documents locale %q, which ValidateLocale rejects: %v", code, err)
		}
	}
	if len(documented) == 0 {
		t.Fatal("no locales read from the table; the column lookup is broken")
	}

	for _, code := range supportedLocales {
		if !documented[code] {
			t.Errorf("locale %q is supported and has no row in the manual's locale table", code)
		}
	}
}

// The prose above the table says "any of the 15 supported locales". A count
// stated in prose beside the list it counts is the cheapest kind of drift —
// both documents once said "Thirteen operators" above fourteen rows.
func TestDocTables_localeCountMatchesTheTable(t *testing.T) {
	_, rows, err := docs.TableUnder("MANUAL.md", "locales", "Locale", "Language / Region",
		"Name format", "Phone format")
	if err != nil {
		t.Fatalf("locale table: %v", err)
	}
	if len(rows) != len(supportedLocales) {
		t.Errorf("the locale table has %d rows; the binary supports %d",
			len(rows), len(supportedLocales))
	}

	data, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}
	if !strings.Contains(data, "15 supported\nlocales") && !strings.Contains(data, "15 supported locales") {
		t.Errorf("the manual no longer says how many locales there are; it should say %d",
			len(supportedLocales))
	}
	if len(supportedLocales) != 15 {
		t.Errorf("the binary supports %d locales and the manual's prose says 15",
			len(supportedLocales))
	}
}

// Locale-awareness is the claim the table's Name format column illustrates:
// asking for a name in one locale must not hand back another's.
func TestDocTables_localePoolsActuallyDiffer(t *testing.T) {
	seen := map[string]string{}
	for _, code := range supportedLocales {
		seed := int64(7)
		r := NewRegistry(&seed, WithLocale(code))
		name, err := r.Evaluate("faker.fullName", nil, map[string]string{})
		if err != nil {
			t.Errorf("faker.fullName under locale %q: %v", code, err)
			continue
		}
		if name == "" {
			t.Errorf("faker.fullName under locale %q returned nothing", code)
		}
		seen[code] = name
	}
	// Not every locale need differ from every other — several share the Latin
	// pools — but a table of fifteen locales that all produce one name is not a
	// locale-aware generator.
	distinct := map[string]bool{}
	for _, name := range seen {
		distinct[name] = true
	}
	if len(distinct) < 2 {
		t.Errorf("all %d locales produced the same name %v; the pools are not locale-aware",
			len(seen), seen)
	}
}

// Every row of the layout table is a Go layout and the exact text it renders
// for one instant. The instant is the one the Example output column uses.
func TestDocTables_dateLayoutsRenderWhatIsDocumented(t *testing.T) {
	hdr, rows, err := docs.Table("MANUAL.md", "Layout", "Renders", "Example output")
	if err != nil {
		t.Fatalf("layout table: %v", err)
	}
	layoutCol, exampleCol := docs.Column(hdr, "Layout"), docs.Column(hdr, "Example output")
	if layoutCol < 0 || exampleCol < 0 {
		t.Fatalf("layout table lost a column: %v", hdr)
	}

	// The instant every Example output column renders, and the one the RFC3339
	// row prints back verbatim.
	const instant = "2024-04-21T15:10:01Z"

	checked := 0
	for _, row := range rows {
		if len(row) <= exampleCol {
			continue
		}
		layout := strings.Trim(row[layoutCol], "`")
		want := strings.Trim(row[exampleCol], "`")
		if layout == "" || want == "" {
			continue
		}

		got, formatErr := formatDate(instant, layout)
		if formatErr != nil {
			t.Errorf("layout %q: %v", layout, formatErr)
			continue
		}
		checked++
		if got != want {
			t.Errorf("layout %q documents %q, renders %q for %s", layout, want, got, instant)
		}
	}
	if checked == 0 {
		t.Fatal("no layouts evaluated; the table moved or its header changed")
	}
}
