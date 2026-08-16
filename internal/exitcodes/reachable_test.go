package exitcodes

import (
	"errors"
	"fmt"
	"testing"
)

func TestReachable(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		root    string
		want    []int
		wantErr error
	}{
		{"map_literal_key_is_not_a_return", "testdata/mapliteral", "run", []int{0, 1}, nil},
		{"map_literal_value_is_not_a_return", "testdata/mapliteral", "run", []int{0, 1}, nil},
		{"helper_called_in_return_position_included", "testdata/callchain", "run", []int{0, 1, 3}, nil},
		{"code_via_local_assignment_included", "testdata/identflow", "run", []int{0, 2, 5}, nil},
		{"int_helper_outside_return_position_excluded", "testdata/nonexit", "run", []int{0, 1}, nil},
		{"unknown_root_is_an_error", "testdata/callchain", "nosuchfunc", nil, ErrRootNotFound},
		{"empty_package_is_an_error", "testdata/empty", "run", nil, ErrNoSources},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			codes, err := Reachable(tc.dir, tc.root)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Reachable(%q, %q) error = %v, want %v", tc.dir, tc.root, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Reachable(%q, %q) unexpected error: %v", tc.dir, tc.root, err)
			}
			got := Set(codes)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("Reachable(%q, %q) = %v, want %v (raw: %+v)", tc.dir, tc.root, got, tc.want, codes)
			}
		})
	}
}

// TestReachable_depth_is_recorded_for_transitive_codes asserts that a code
// found only by following a call (directly or through a local identifier) is
// stamped with a nonzero Depth, and a code the root returns itself is
// stamped with Depth 0. Depth is what MaxDepth uses to detect a walk whose
// call resolution silently stopped working.
func TestReachable_depth_is_recorded_for_transitive_codes(t *testing.T) {
	codes, err := Reachable("testdata/identflow", "run")
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}

	zero, ok := Find(codes, 0)
	if !ok {
		t.Fatalf("code 0 not found in %+v", codes)
	}
	if zero.Depth != 0 {
		t.Errorf("code 0 (returned directly by run) has Depth %d, want 0", zero.Depth)
	}

	two, ok := Find(codes, 2)
	if !ok {
		t.Fatalf("code 2 not found in %+v", codes)
	}
	if two.Depth == 0 {
		t.Errorf("code 2 (returned by classify, reached via a local identifier) has Depth 0, want > 0")
	}
	if two.Fn != "classify" {
		t.Errorf("code 2's Fn = %q, want %q", two.Fn, "classify")
	}

	if got := MaxDepth(codes); got == 0 {
		t.Errorf("MaxDepth(codes) = 0; a return-position call was resolved, so this must be > 0")
	}
}

// TestReachable_ignores_test_files asserts _test.go files in dir are not
// parsed. A codePresentOnlyInTests function is deliberately not offered as
// dead code fixtures do not carry _test.go siblings.
func TestReachable_ignores_test_files(t *testing.T) {
	// testdata/callchain has no _test.go file, but this test pins the
	// contract at the parseDir level via a directory that does carry one:
	// reuse testdata/callchain and add nothing — the guarantee under test is
	// that Reachable never errors trying to parse *_test.go, and that a
	// package consisting ONLY of a _test.go file is treated as having no
	// sources. testdata/empty (no .go files at all) already proves the
	// "nothing to parse" half; this proves the "test files don't count as
	// sources" half using a dedicated fixture.
	codes, err := Reachable("testdata/onlytest", "run")
	if !errors.Is(err, ErrNoSources) {
		t.Fatalf("Reachable(%q) error = %v (codes: %+v), want ErrNoSources", "testdata/onlytest", err, codes)
	}
}

func TestSet_dedupesAndSorts(t *testing.T) {
	codes := []Code{{Value: 5}, {Value: 1}, {Value: 5}, {Value: 0}}
	got := fmt.Sprint(Set(codes))
	want := fmt.Sprint([]int{0, 1, 5})
	if got != want {
		t.Errorf("Set(...) = %s, want %s", got, want)
	}
}

func TestFind_returnsProvenance(t *testing.T) {
	codes, err := Reachable("testdata/callchain", "run")
	if err != nil {
		t.Fatalf("Reachable: %v", err)
	}
	c, ok := Find(codes, 3)
	if !ok {
		t.Fatalf("Find(codes, 3) not found in %+v", codes)
	}
	if c.Fn != "helper" {
		t.Errorf("Find(codes, 3).Fn = %q, want %q", c.Fn, "helper")
	}
	if c.File == "" {
		t.Error("Find(codes, 3).File is empty; provenance must name a file")
	}
	if c.Line == 0 {
		t.Error("Find(codes, 3).Line is 0; provenance must name a line")
	}

	if _, ok := Find(codes, 999); ok {
		t.Error("Find(codes, 999) reported found for a value not present")
	}
}

func TestMaxDepth_emptyIsZero(t *testing.T) {
	if got := MaxDepth(nil); got != 0 {
		t.Errorf("MaxDepth(nil) = %d, want 0", got)
	}
}
