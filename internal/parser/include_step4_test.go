package parser

import (
	"path/filepath"
	"testing"
)

// Step 4 tests: ParseFile wired with resolveIncludes.

func TestParseFile_include_parses_and_returns_both_items(t *testing.T) {
	// ParseFile on parent_simple.yaml succeeds and returns spliced items.
	col, err := ParseFile("testdata/include/parent_simple.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) != 2 {
		t.Errorf("Requests.Items len = %d, want 2 (parent + child)", len(col.Requests.Items))
	}
}

func TestParseFile_include_resolves_and_splices(t *testing.T) {
	// End-to-end through ParseFile: parent + child produce the merged
	// request list with parent's order preserved and includes spliced after.
	col, err := ParseFile("testdata/include/parent_simple.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Parent's own request is first, then the included child.
	if len(col.Requests.Items) < 2 {
		t.Fatalf("expected >= 2 requests, got %d", len(col.Requests.Items))
	}
	if col.Requests.Items[0].Name != "Parent Request" {
		t.Errorf("Requests.Items[0].Name = %q, want Parent Request", col.Requests.Items[0].Name)
	}
	if col.Requests.Items[1].Name != "Child Request" {
		t.Errorf("Requests.Items[1].Name = %q, want Child Request", col.Requests.Items[1].Name)
	}
}

func TestParseFile_include_path_relative_to_parent_not_cwd(t *testing.T) {
	// Parse parent in subdir/ using its absolute path; include path
	// "./../child_simple.yaml" must resolve relative to the parent file's
	// directory (subdir/), not the process CWD.
	//
	// We change CWD to a temp dir to prove include resolution is
	// parent-relative, not CWD-relative.

	// Resolve the absolute path BEFORE changing CWD, while the package dir
	// is still the working directory.
	absParent, err := filepath.Abs("testdata/include/subdir/parent_relative.yaml")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	t.Chdir(t.TempDir()) // CWD is now an unrelated directory

	// Use absolute path to open the parent file itself (CWD is wrong).
	col, err := ParseFile(absParent)
	if err != nil {
		t.Fatalf("ParseFile with absolute path: %v", err)
	}

	// child_simple.yaml has one request; parent has one own request.
	if len(col.Requests.Items) != 2 {
		t.Errorf("Requests.Items len = %d, want 2 (parent + included child)", len(col.Requests.Items))
	}
}

func TestParseFile_include_external_files_tracked(t *testing.T) {
	// After ParseFile with includes, ExternalFiles should contain the
	// absolute path of the included child file.
	col, err := ParseFile("testdata/include/parent_simple.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.ExternalFiles) == 0 {
		t.Errorf("ExternalFiles should contain the included child path, got empty slice")
	}
}
