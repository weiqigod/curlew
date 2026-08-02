package parser

import (
	"errors"
	"testing"
)

// TestResolveIncludes exercises resolveIncludes via ParseFile (no gate)
// on pre-built testdata fixtures.
func TestResolveIncludes(t *testing.T) {
	tests := []struct {
		name            string
		parentFile      string
		wantErr         error
		wantTotalItems  int               // total request items across Setup+Requests+Teardown after splice
		wantRequestLen  int               // only Requests.Items length (0 = skip check)
		wantOverrides   map[string]string // expected Variables.Values on first child (spliced) request
		checkNoOverride bool              // assert that the first spliced item has no overrides
	}{
		{
			name:           "single include splices child requests in order",
			parentFile:     "testdata/include/parent_simple.yaml",
			wantRequestLen: 2, // 1 child spliced before parent's own (child is first)
		},
		{
			name:          "child variable overrides parent for child request only",
			parentFile:    "testdata/include/parent_override.yaml",
			wantOverrides: map[string]string{"b": "99", "c": "3"},
		},
		{
			name:            "child with no vars resolves base_url from parent snapshot",
			parentFile:      "testdata/include/parent_base_url.yaml",
			checkNoOverride: true, // child declared no vars; nil overrides → nothing stamped
		},
		{
			name:           "transitive include resolves with cumulative overrides",
			parentFile:     "testdata/include/parent_grandchild.yaml",
			wantRequestLen: 3, // parent + child + grandchild
		},
		{
			name:       "circular include detected",
			parentFile: "testdata/include/circular_a.yaml",
			wantErr:    ErrCircularInclude,
		},
		{
			name:           "include relative path resolved from including file",
			parentFile:     "testdata/include/subdir/parent_relative.yaml",
			wantRequestLen: 2, // child_simple spliced + parent's own
		},
		{
			name:       "include file not found",
			parentFile: "testdata/include/parent_missing.yaml",
			wantErr:    ErrIncludeNotFound,
		},
		{
			name:          "child request own override wins over include overrides",
			parentFile:    "testdata/include/parent_request_override.yaml",
			wantOverrides: map[string]string{"b": "req_b_wins"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.parentFile)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want error wrapping %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile(%q): %v", tt.parentFile, err)
			}

			total := len(col.Setup.Items) + len(col.Requests.Items) + len(col.Teardown.Items)
			if tt.wantTotalItems > 0 && total != tt.wantTotalItems {
				t.Errorf("total items = %d, want %d", total, tt.wantTotalItems)
			}
			if tt.wantRequestLen > 0 && len(col.Requests.Items) != tt.wantRequestLen {
				t.Errorf("Requests.Items len = %d, want %d", len(col.Requests.Items), tt.wantRequestLen)
			}

			// Find first spliced (child) request — child items are appended AFTER
			// parent's own requests. For our test fixtures with one parent request
			// and one included child, the child is at index 1.
			if tt.wantOverrides != nil && len(col.Requests.Items) >= 2 {
				// Last item is the included child (appended after parent's own).
				item := col.Requests.Items[len(col.Requests.Items)-1]
				for k, want := range tt.wantOverrides {
					got, ok := item.Variables.Values[k]
					if !ok {
						t.Errorf("item.Variables.Values missing key %q", k)
					} else if got != want {
						t.Errorf("item.Variables.Values[%q] = %q, want %q", k, got, want)
					}
				}
			}

			if tt.checkNoOverride && len(col.Requests.Items) >= 2 {
				// Last item is the included child (appended after parent's own).
				item := col.Requests.Items[len(col.Requests.Items)-1]
				if len(item.Variables.Values) != 0 {
					t.Errorf("expected no override vars on spliced child item, got %v", item.Variables.Values)
				}
			}
		})
	}
}

func TestResolveIncludes_parent_requests_unchanged_by_child_overrides(t *testing.T) {
	// parent has variables {a:1, b:2} and one own request "Parent Unchanged Request".
	// child has variables {b:99, c:3} and one request "Child Override Request".
	// After resolveIncludes:
	//   - Child request (Requests.Items[0]) must have overrides b=99, c=3.
	//   - Parent request (Requests.Items[1]) must have NO overrides stamped.
	col, err := ParseFile("testdata/include/parent_unchanged.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 2 {
		t.Fatalf("expected 2 request items, got %d", len(col.Requests.Items))
	}

	// Parent's own request is at [0], child (appended) is at [1].
	parentReq := col.Requests.Items[0]
	if len(parentReq.Variables.Values) != 0 {
		t.Errorf("parent req should have no stamped overrides, got %v", parentReq.Variables.Values)
	}

	childReq := col.Requests.Items[1]
	if childReq.Variables.Values["b"] != "99" {
		t.Errorf("child req b = %q, want 99", childReq.Variables.Values["b"])
	}
	if childReq.Variables.Values["c"] != "3" {
		t.Errorf("child req c = %q, want 3", childReq.Variables.Values["c"])
	}
}

func TestResolveIncludes_setup_and_teardown_spliced(t *testing.T) {
	// Behavior 1: "setup/requests/teardown items are appended in include order".
	// parent_setup_teardown.yaml has its own setup, requests, and teardown sections.
	// child_setup_teardown.yaml also has setup, requests, and teardown sections.
	// After resolveIncludes, the child's items must be spliced into EACH section.
	col, err := ParseFile("testdata/include/parent_setup_teardown.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Setup: parent has 1 own entry, child has 1 entry → total 2.
	if len(col.Setup.Items) != 2 {
		t.Errorf("Setup.Items len = %d, want 2 (parent + child)", len(col.Setup.Items))
	}
	// Parent setup is own (first), child setup is spliced (appended).
	if len(col.Setup.Items) >= 1 && col.Setup.Items[0].Name != "Parent Setup Step" {
		t.Errorf("Setup.Items[0].Name = %q, want Parent Setup Step", col.Setup.Items[0].Name)
	}
	if len(col.Setup.Items) >= 2 && col.Setup.Items[1].Name != "Child Setup Step" {
		t.Errorf("Setup.Items[1].Name = %q, want Child Setup Step", col.Setup.Items[1].Name)
	}

	// Requests: same pattern.
	if len(col.Requests.Items) != 2 {
		t.Errorf("Requests.Items len = %d, want 2 (parent + child)", len(col.Requests.Items))
	}

	// Teardown: parent has 1 own entry, child has 1 entry → total 2.
	if len(col.Teardown.Items) != 2 {
		t.Errorf("Teardown.Items len = %d, want 2 (parent + child)", len(col.Teardown.Items))
	}
	if len(col.Teardown.Items) >= 1 && col.Teardown.Items[0].Name != "Parent Teardown Step" {
		t.Errorf("Teardown.Items[0].Name = %q, want Parent Teardown Step", col.Teardown.Items[0].Name)
	}
	if len(col.Teardown.Items) >= 2 && col.Teardown.Items[1].Name != "Child Teardown Step" {
		t.Errorf("Teardown.Items[1].Name = %q, want Child Teardown Step", col.Teardown.Items[1].Name)
	}
}

func TestResolveIncludes_grandchild_sees_child_overrides(t *testing.T) {
	// parent vars: {a:1, b:2}
	// child vars:  {b:99, c:3}
	// grandchild vars: {d:4}
	// After resolution, the grandchild request's overrides must contain
	// b=99, c=3, d=4 (grandchild sees child's override of b plus its own d).
	col, err := ParseFile("testdata/include/parent_grandchild.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Expected order of Requests.Items:
	//   [0] grandchild (deepest, spliced first via child → grandchild recursion)
	//   [1] child own request (child_grandchild.yaml's own request)
	//   [2] parent own request
	if len(col.Requests.Items) != 3 {
		t.Fatalf("expected 3 request items, got %d: %v", len(col.Requests.Items),
			func() []string {
				ns := make([]string, len(col.Requests.Items))
				for i, r := range col.Requests.Items {
					ns[i] = r.Name
				}
				return ns
			}())
	}

	// Find grandchild request by name.
	var grandchildItem *RequestItem
	for i := range col.Requests.Items {
		if col.Requests.Items[i].Name == "Grandchild Request" {
			grandchildItem = &col.Requests.Items[i]
			break
		}
	}
	if grandchildItem == nil {
		t.Fatal("Grandchild Request not found in spliced items")
	}

	for k, want := range map[string]string{"b": "99", "c": "3", "d": "4"} {
		got, ok := grandchildItem.Variables.Values[k]
		if !ok {
			t.Errorf("grandchild request missing key %q", k)
		} else if got != want {
			t.Errorf("grandchild request Variables.Values[%q] = %q, want %q", k, got, want)
		}
	}
}
