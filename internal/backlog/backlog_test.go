package backlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixtures ---------------------------------------------------------------
//
// Each failure case starts from a valid miniature repository and breaks
// exactly one thing, so a passing case proves that one guard bites rather
// than that the fixture was malformed in some other way.

const validBacklog = `schema_version: '2.0'
project: 'Curlew'
completed:
  - id: M1-001
    completed_date: 2026-03-10
    verified_by: ai
capabilities:
  first_test:
    description: 'From zero to a running test'
    milestone: 'M1'
    tasks:
      - M1-001
      - id: M1-002
        status: done
        branch: feature/M1-002-thing
        planned_date: 2026-03-10
        completed_date: 2026-03-10
        verified_by: ai
`

const validTask1 = `id: M1-001
title: "First task"
status: done
`

const validTask2 = `id: M1-002
title: "Second task"
status: done
`

type fixture struct {
	backlog string
	tasks   map[string]string
}

func validFixture() fixture {
	return fixture{
		backlog: validBacklog,
		tasks: map[string]string{
			"M1-001.yaml": validTask1,
			"M1-002.yaml": validTask2,
		},
	}
}

// write materialises the fixture and returns its root.
func (f fixture) write(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tasksDir := filepath.Join(root, "management", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backlogPath := filepath.Join(root, "management", "backlog.yaml")
	if err := os.WriteFile(backlogPath, []byte(f.backlog), 0o644); err != nil {
		t.Fatalf("write backlog: %v", err)
	}
	for name, body := range f.tasks {
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write task %s: %v", name, err)
		}
	}
	return root
}

// --- the guard --------------------------------------------------------------

// TestBacklog_valid_fixture_loads is the control. If this fails, every
// failure case below is meaningless.
func TestBacklog_valid_fixture_loads(t *testing.T) {
	b, err := Load(validFixture().write(t))
	if err != nil {
		t.Fatalf("valid fixture should load, got: %v", err)
	}
	if got, want := len(b.Tasks), 2; got != want {
		t.Errorf("tasks = %d, want %d", got, want)
	}
	if got, want := b.Capabilities, 1; got != want {
		t.Errorf("capabilities = %d, want %d", got, want)
	}
	for _, task := range b.Tasks {
		if task.Status != StatusDone {
			t.Errorf("%s: status = %q, want done", task.ID, task.Status)
		}
		if task.Capability != "first_test" || task.Milestone != "M1" {
			t.Errorf("%s: capability/milestone = %q/%q", task.ID, task.Capability, task.Milestone)
		}
	}
}

// TestBacklog_structural_surprises_are_errors is the heart of this package.
// Every case is a way a traversal could read less than it thought it had.
// None of them may produce a nil error, because a nil error is read as
// "the backlog is clean".
func TestBacklog_structural_surprises_are_errors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*fixture)
		wantErr string // substring the message must contain
	}{
		{
			name: "no capabilities at all",
			mutate: func(f *fixture) {
				f.backlog = f.backlog[:strings.Index(f.backlog, "capabilities:")] + "capabilities: {}\n"
				f.tasks = map[string]string{}
			},
			wantErr: "no capabilities",
		},
		{
			name: "capabilities key renamed",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "capabilities:", "capabilties:", 1)
			},
			wantErr: "capabilties",
		},
		{
			name: "capability has no tasks",
			mutate: func(f *fixture) {
				f.backlog = validBacklog[:strings.Index(validBacklog, "    tasks:")] + "    tasks: []\n"
				f.tasks = map[string]string{}
			},
			wantErr: "no tasks",
		},
		{
			name: "task file does not parse",
			mutate: func(f *fixture) {
				f.tasks["M1-002.yaml"] = validTask2 + "notes:\n  - `backticks` cannot open a scalar\n"
			},
			wantErr: "M1-002.yaml",
		},
		{
			// Deletes M1-001, the bare-id entry. Deleting M1-002 instead
			// would trip the disagreement guard (its backlog status versus
			// the zero value) and pass even without a missing-file check.
			name:    "backlog names a task with no file",
			mutate:  func(f *fixture) { delete(f.tasks, "M1-001.yaml") },
			wantErr: "does not exist",
		},
		{
			// A duplicate id would otherwise collapse in the index map,
			// silently lowering the task count -- the same "read less than
			// you think" failure this package exists to prevent.
			name: "the same task is listed twice",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "      - M1-001\n", "      - M1-001\n      - M1-001\n", 1)
			},
			wantErr: "listed twice",
		},
		{
			name: "capability has no milestone",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "    milestone: 'M1'\n", "", 1)
			},
			wantErr: "no milestone",
		},
		{
			// Redundant with the missing-file check on safety, but not on
			// clarity: this reports the directory as empty rather than
			// reporting the first missing id and leaving the reader to
			// discover that none of them are there.
			name:    "task directory is empty",
			mutate:  func(f *fixture) { f.tasks = map[string]string{} },
			wantErr: "no task files",
		},
		{
			// The dangerous combination: a file that neither parses nor is
			// listed. If parse failures were skipped rather than fatal, the
			// reconciliation checks could not see this file either -- it is
			// absent from both sides -- and Load would return a clean result.
			name: "unparseable task file that the backlog does not list",
			mutate: func(f *fixture) {
				f.tasks["M9-999.yaml"] = "id: M9-999\ntitle: \"Broken\"\nstatus: done\n" +
					"notes:\n  - `backticks` cannot open a scalar\n"
			},
			wantErr: "M9-999.yaml",
		},
		{
			name: "task file absent from the backlog",
			mutate: func(f *fixture) {
				f.tasks["M9-999.yaml"] = "id: M9-999\ntitle: \"Orphan\"\nstatus: done\n"
			},
			wantErr: "M9-999",
		},
		{
			name:    "task file has no status",
			mutate:  func(f *fixture) { f.tasks["M1-002.yaml"] = "id: M1-002\ntitle: \"No status\"\n" },
			wantErr: "has no status",
		},
		{
			// Targets M1-001, whose backlog entry is a bare id carrying no
			// status. Using M1-002 instead would trip the disagreement guard
			// first and pass even if status validation were removed entirely.
			name: "task file has an unknown status",
			mutate: func(f *fixture) {
				f.tasks["M1-001.yaml"] = strings.Replace(validTask1, "status: done", "status: finished", 1)
			},
			wantErr: "unknown status",
		},
		{
			name: "backlog and task file disagree about status",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "        status: done", "        status: review", 1)
			},
			wantErr: "disagree",
		},
		{
			name: "task id does not match its filename",
			mutate: func(f *fixture) {
				f.tasks["M1-002.yaml"] = strings.Replace(validTask2, "id: M1-002", "id: M1-003", 1)
			},
			wantErr: "filename",
		},
		{
			name: "task entry is neither an id nor a mapping",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "      - M1-001", "      - [nested, list]", 1)
			},
			// Deliberately specific: an entry of an unexpected shape must be
			// rejected on its shape. Falling through to the empty-id guard
			// would also error, but for the wrong reason, so a loose
			// substring here would let that regression pass.
			wantErr: "want a task id or a mapping",
		},
		{
			name: "task entry is an empty id",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "      - M1-001", "      - ''", 1)
			},
			wantErr: "empty",
		},
		{
			name: "mapping task entry has no id",
			mutate: func(f *fixture) {
				f.backlog = strings.Replace(f.backlog, "      - id: M1-002", "      - noid: M1-002", 1)
			},
			wantErr: "missing an id",
		},
		{
			name:    "management directory is missing entirely",
			mutate:  func(f *fixture) { f.backlog = "" },
			wantErr: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := validFixture()
			tc.mutate(&f)
			root := f.write(t)
			if tc.name == "management directory is missing entirely" {
				if err := os.RemoveAll(filepath.Join(root, "management")); err != nil {
					t.Fatalf("remove: %v", err)
				}
			}

			b, err := Load(root)
			if err == nil {
				t.Fatalf("Load returned nil error — a false clear. Got %d tasks, %d capabilities",
					len(b.Tasks), b.Capabilities)
			}
			if tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not mention %q — the message must name the offending item", err, tc.wantErr)
			}
		})
	}
}

// TestBacklog_empty_result_is_never_a_success asserts the invariant directly:
// there is no input for which Load succeeds while having read nothing.
func TestBacklog_empty_result_is_never_a_success(t *testing.T) {
	roots := map[string]func(t *testing.T) string{
		"empty directory": func(t *testing.T) string { return t.TempDir() },
		"empty backlog file": func(t *testing.T) string {
			f := validFixture()
			f.backlog = ""
			f.tasks = map[string]string{}
			return f.write(t)
		},
		"backlog with only metadata": func(t *testing.T) string {
			f := validFixture()
			f.backlog = "schema_version: '2.0'\nproject: 'Curlew'\n"
			f.tasks = map[string]string{}
			return f.write(t)
		},
	}
	for name, mk := range roots {
		t.Run(name, func(t *testing.T) {
			b, err := Load(mk(t))
			if err == nil && (b == nil || len(b.Tasks) == 0) {
				t.Fatal("Load succeeded having read zero tasks — this is the false clear this package exists to prevent")
			}
		})
	}
}

// --- the live repository ----------------------------------------------------

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return root
}

// TestBacklog_repository_is_consistent runs the loader against the real
// management/ directory. It fails on any drift between backlog.yaml and the
// task files, including a task file that does not parse.
func TestBacklog_repository_is_consistent(t *testing.T) {
	b, err := Load(repoRoot(t))
	if err != nil {
		t.Fatalf("management/ is inconsistent: %v", err)
	}
	if len(b.Tasks) == 0 {
		t.Fatal("loaded zero tasks from the real repository")
	}
	t.Logf("%d tasks across %d capabilities", len(b.Tasks), b.Capabilities)
	for status, n := range b.CountByStatus() {
		t.Logf("  %s: %d", status, n)
	}
	if open := b.Open(); len(open) > 0 {
		for _, task := range open {
			t.Logf("  OPEN %s (%s) %s", task.ID, task.Status, task.Capability)
		}
	}
}
