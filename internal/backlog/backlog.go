// Package backlog loads and validates the task index in management/.
//
// The point of this package is that it cannot report "nothing open" by
// accident. Every ad-hoc traversal of management/backlog.yaml written so far
// has had the same failure mode: it walked a key path that did not exist,
// found zero tasks, and reported a clean backlog. A checker that answers
// "clear" when it has in fact read nothing is worse than no checker, because
// it converts an unread file into a green light.
//
// So Load treats every structural surprise as an error rather than as an
// empty result: an unknown key, a capability with no tasks, a task file that
// does not parse, an id present in one source but not the other, or a status
// outside the documented lifecycle. The only way to get a nil error back is
// to have actually read and reconciled both sources.
//
// Both sources matter. backlog.yaml lists a task either as a bare id or as a
// mapping that carries its own status, and management/tasks/<ID>.yaml carries
// the authoritative record. Where both state a status they must agree; a
// checker that consults only one of them can be defeated by editing the other.
package backlog

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Status is a point in the task lifecycle documented in CLAUDE.md:
// backlog -> planned -> in_progress -> review -> done.
type Status string

// The complete set of statuses a task may hold. Anything else is an error;
// an unrecognised status must never be silently treated as done.
const (
	StatusBacklog    Status = "backlog"
	StatusPlanned    Status = "planned"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusDone       Status = "done"
)

var knownStatuses = []Status{
	StatusBacklog, StatusPlanned, StatusInProgress, StatusReview, StatusDone,
}

// Task is one unit of work, reconciled from both sources.
type Task struct {
	ID         string
	Status     Status
	Capability string
	Milestone  string
}

// Backlog is the reconciled contents of management/.
type Backlog struct {
	Capabilities int
	Tasks        []Task
}

// Open returns every task not yet done, in id order.
func (b *Backlog) Open() []Task {
	var open []Task
	for _, task := range b.Tasks {
		if task.Status != StatusDone {
			open = append(open, task)
		}
	}
	return open
}

// CountByStatus tallies tasks by lifecycle status.
func (b *Backlog) CountByStatus() map[Status]int {
	counts := make(map[Status]int, len(knownStatuses))
	for _, task := range b.Tasks {
		counts[task.Status]++
	}
	return counts
}

// --- on-disk shapes ---------------------------------------------------------

// fileBacklog is decoded with KnownFields(true). backlog.yaml is small and
// hand-maintained, so an unrecognised key is far more likely to be a typo
// that would silently hide tasks than a deliberate extension.
type fileBacklog struct {
	SchemaVersion string                    `yaml:"schema_version"`
	Project       string                    `yaml:"project"`
	Completed     []completedEntry          `yaml:"completed"`
	Capabilities  map[string]fileCapability `yaml:"capabilities"`
}

type completedEntry struct {
	ID            string `yaml:"id"`
	CompletedDate string `yaml:"completed_date"`
	VerifiedBy    string `yaml:"verified_by"`
	MergedPR      int    `yaml:"merged_pr"`
}

type fileCapability struct {
	Description string      `yaml:"description"`
	Milestone   string      `yaml:"milestone"`
	Tasks       []taskEntry `yaml:"tasks"`
}

// taskEntry is either a bare id or a mapping carrying a status. Anything
// else is rejected outright rather than decoding to a zero value, which is
// how a nested list or a stray null would otherwise vanish from the count.
type taskEntry struct {
	ID        string
	Status    Status
	hasStatus bool
}

func (e *taskEntry) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var id string
		if err := node.Decode(&id); err != nil {
			return fmt.Errorf("task entry: %w", err)
		}
		e.ID = strings.TrimSpace(id)
		return nil
	case yaml.MappingNode:
		var m struct {
			ID     string `yaml:"id"`
			Status string `yaml:"status"`
		}
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("task entry: %w", err)
		}
		e.ID = strings.TrimSpace(m.ID)
		e.Status = Status(strings.TrimSpace(m.Status))
		e.hasStatus = e.Status != ""
		return nil
	default:
		return fmt.Errorf("line %d: task entry is %s, want a task id or a mapping",
			node.Line, kindName(node.Kind))
	}
}

func kindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "a sequence"
	case yaml.MappingNode:
		return "a mapping"
	case yaml.ScalarNode:
		return "a scalar"
	case yaml.AliasNode:
		return "an alias"
	case yaml.DocumentNode:
		return "a document"
	default:
		return "an unrecognised node"
	}
}

// fileTask is the task file. It is decoded leniently — task files carry many
// optional keys and gain more over time — but the three fields below must be
// present and well-formed.
type fileTask struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Status string `yaml:"status"`
}

// --- loading ----------------------------------------------------------------

// Load reads and validates the backlog rooted at the repository root. It
// returns an error, never an empty success, when either source cannot be
// fully read and reconciled.
func Load(root string) (*Backlog, error) {
	doc, err := loadIndex(root)
	if err != nil {
		return nil, err
	}

	indexed, err := indexTasks(doc)
	if err != nil {
		return nil, err
	}

	onDisk, err := loadTaskFiles(root)
	if err != nil {
		return nil, err
	}

	tasks, err := reconcile(indexed, onDisk)
	if err != nil {
		return nil, err
	}

	return &Backlog{Capabilities: len(doc.Capabilities), Tasks: tasks}, nil
}

func loadIndex(root string) (*fileBacklog, error) {
	path := filepath.Join(root, "management", "backlog.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the backlog index: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var doc fileBacklog
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("management/backlog.yaml is empty")
		}
		return nil, fmt.Errorf("management/backlog.yaml: %w", err)
	}
	if len(doc.Capabilities) == 0 {
		return nil, fmt.Errorf("management/backlog.yaml declares no capabilities")
	}
	return &doc, nil
}

// indexedTask is what backlog.yaml claims about a task.
type indexedTask struct {
	Capability string
	Milestone  string
	Status     Status
	hasStatus  bool
}

func indexTasks(doc *fileBacklog) (map[string]indexedTask, error) {
	names := make([]string, 0, len(doc.Capabilities))
	for name := range doc.Capabilities {
		names = append(names, name)
	}
	sort.Strings(names)

	indexed := make(map[string]indexedTask)
	for _, name := range names {
		cap := doc.Capabilities[name]
		if len(cap.Tasks) == 0 {
			return nil, fmt.Errorf("capability %q has no tasks", name)
		}
		if strings.TrimSpace(cap.Milestone) == "" {
			return nil, fmt.Errorf("capability %q has no milestone", name)
		}
		for i, entry := range cap.Tasks {
			switch {
			case entry.ID == "" && entry.hasStatus:
				return nil, fmt.Errorf("capability %q: task entry %d is missing an id", name, i)
			case entry.ID == "":
				return nil, fmt.Errorf("capability %q: task entry %d has an empty id", name, i)
			}
			if prev, dup := indexed[entry.ID]; dup {
				return nil, fmt.Errorf("task %s is listed twice: in %q and in %q",
					entry.ID, prev.Capability, name)
			}
			if entry.hasStatus {
				if err := validateStatus(entry.Status, "management/backlog.yaml entry "+entry.ID); err != nil {
					return nil, err
				}
			}
			indexed[entry.ID] = indexedTask{
				Capability: name,
				Milestone:  cap.Milestone,
				Status:     entry.Status,
				hasStatus:  entry.hasStatus,
			}
		}
	}
	return indexed, nil
}

func loadTaskFiles(root string) (map[string]Status, error) {
	dir := filepath.Join(root, "management", "tasks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the task directory: %w", err)
	}

	onDisk := make(map[string]Status)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		rel := filepath.Join("management", "tasks", name)

		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		var task fileTask
		if err := yaml.Unmarshal(raw, &task); err != nil {
			return nil, fmt.Errorf("%s does not parse as YAML: %w", rel, err)
		}
		if strings.TrimSpace(task.ID) == "" {
			return nil, fmt.Errorf("%s has no id", rel)
		}
		if stem := strings.TrimSuffix(name, ".yaml"); task.ID != stem {
			return nil, fmt.Errorf("%s: id %q does not match its filename", rel, task.ID)
		}
		if strings.TrimSpace(task.Status) == "" {
			return nil, fmt.Errorf("%s has no status", rel)
		}
		if err := validateStatus(Status(task.Status), rel); err != nil {
			return nil, err
		}
		onDisk[task.ID] = Status(task.Status)
	}

	if len(onDisk) == 0 {
		return nil, fmt.Errorf("management/tasks contains no task files")
	}
	return onDisk, nil
}

func reconcile(indexed map[string]indexedTask, onDisk map[string]Status) ([]Task, error) {
	ids := make([]string, 0, len(indexed))
	for id := range indexed {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	tasks := make([]Task, 0, len(ids))
	for _, id := range ids {
		entry := indexed[id]
		status, ok := onDisk[id]
		if !ok {
			return nil, fmt.Errorf("%s is listed in management/backlog.yaml but management/tasks/%s.yaml does not exist", id, id)
		}
		if entry.hasStatus && entry.Status != status {
			return nil, fmt.Errorf("%s: backlog.yaml says %q but management/tasks/%s.yaml says %q — the two sources disagree",
				id, entry.Status, id, status)
		}
		tasks = append(tasks, Task{
			ID:         id,
			Status:     status,
			Capability: entry.Capability,
			Milestone:  entry.Milestone,
		})
	}

	orphans := make([]string, 0)
	for id := range onDisk {
		if _, ok := indexed[id]; !ok {
			orphans = append(orphans, id)
		}
	}
	if len(orphans) > 0 {
		sort.Strings(orphans)
		return nil, fmt.Errorf("task file(s) %s exist but are not listed in management/backlog.yaml",
			strings.Join(orphans, ", "))
	}

	return tasks, nil
}

func validateStatus(s Status, where string) error {
	for _, known := range knownStatuses {
		if s == known {
			return nil
		}
	}
	return fmt.Errorf("%s: unknown status %q (want one of %s)", where, s, joinStatuses())
}

func joinStatuses() string {
	parts := make([]string, len(knownStatuses))
	for i, s := range knownStatuses {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}
