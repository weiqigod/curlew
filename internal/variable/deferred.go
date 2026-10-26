package variable

import (
	"strings"
	"sync"
)

// Deferred holds variables whose values are fetched on first use, such as
// vault secrets a run may never need. One Deferred is shared by a scope and
// every snapshot or override scope derived from it, so each value is fetched
// at most once per run.
type Deferred struct {
	mu     sync.Mutex
	names  map[string]bool
	fetch  func(name string) (string, error)
	values map[string]string
}

// NewDeferred returns a Deferred for names, calling fetch on first use.
func NewDeferred(names []string, fetch func(name string) (string, error)) *Deferred {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return &Deferred{names: set, fetch: fetch, values: map[string]string{}}
}

func (d *Deferred) has(name string) bool {
	return d != nil && d.names[name]
}

func (d *Deferred) get(name string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if v, ok := d.values[name]; ok {
		return v, nil
	}
	v, err := d.fetch(name)
	if err != nil {
		return "", err
	}
	d.values[name] = v
	return v, nil
}

// referencedIn reports whether s still contains a reference to a deferred name.
func (d *Deferred) referencedIn(s string) bool {
	if d == nil || !strings.Contains(s, "{{") {
		return false
	}
	for _, ref := range findVarRefs(s) {
		if d.names[ref] {
			return true
		}
	}
	return false
}

// WithDeferred returns a shallow copy of the scope whose deferred names stay as
// {{name}} placeholders through Resolve and are fetched when interpolated.
// Call it before Resolve.
func (s *Scope) WithDeferred(d *Deferred) *Scope {
	cp := *s
	cp.deferred = d
	return &cp
}
