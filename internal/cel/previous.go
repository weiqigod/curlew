package cel

import (
	"fmt"
	"sync"

	celgo "github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
)

// parseEnv parses expressions for static inspection. Parsing needs no
// declarations, only the standard macros, so one shared environment serves.
var parseEnv = sync.OnceValues(func() (*celgo.Env, error) {
	return celgo.NewEnv(celgo.StdLib())
})

// UsesPrevious reports whether src reads the `previous` binding. It walks the
// parsed expression, so `previous` inside a string literal or as a field name
// (vars.previous) does not count, and a comprehension variable named previous
// hides the binding inside its loop.
func UsesPrevious(src string) (bool, error) {
	env, err := parseEnv()
	if err != nil {
		return false, err
	}
	ast, issues := env.Parse(src)
	if issues != nil && issues.Err() != nil {
		return false, fmt.Errorf("cel: %w", issues.Err())
	}
	return usesIdent(ast.NativeRep().Expr(), "previous", false), nil
}

// VarsReferences returns the sorted names src reads as vars.<name>, so a
// dependency analysis can order the expression after the request that
// extracts them.
func VarsReferences(src string) ([]string, error) {
	env, err := parseEnv()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Parse(src)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("cel: %w", issues.Err())
	}
	return collectVarsRefs(ast), nil
}

// usesIdent reports whether e reads the free identifier name. hidden is true
// inside a comprehension that binds name itself.
func usesIdent(e celast.Expr, name string, hidden bool) bool {
	if e == nil {
		return false
	}
	switch e.Kind() {
	case celast.IdentKind:
		return !hidden && e.AsIdent() == name
	case celast.SelectKind:
		return usesIdent(e.AsSelect().Operand(), name, hidden)
	case celast.CallKind:
		call := e.AsCall()
		if call.IsMemberFunction() && usesIdent(call.Target(), name, hidden) {
			return true
		}
		for _, arg := range call.Args() {
			if usesIdent(arg, name, hidden) {
				return true
			}
		}
	case celast.ListKind:
		for _, el := range e.AsList().Elements() {
			if usesIdent(el, name, hidden) {
				return true
			}
		}
	case celast.MapKind:
		for _, entry := range e.AsMap().Entries() {
			me := entry.AsMapEntry()
			if usesIdent(me.Key(), name, hidden) || usesIdent(me.Value(), name, hidden) {
				return true
			}
		}
	case celast.StructKind:
		for _, field := range e.AsStruct().Fields() {
			if usesIdent(field.AsStructField().Value(), name, hidden) {
				return true
			}
		}
	case celast.ComprehensionKind:
		comp := e.AsComprehension()
		if usesIdent(comp.IterRange(), name, hidden) || usesIdent(comp.AccuInit(), name, hidden) {
			return true
		}
		inLoop := hidden || comp.IterVar() == name || comp.IterVar2() == name || comp.AccuVar() == name
		if usesIdent(comp.LoopCondition(), name, inLoop) || usesIdent(comp.LoopStep(), name, inLoop) {
			return true
		}
		return usesIdent(comp.Result(), name, hidden || comp.AccuVar() == name)
	}
	return false
}

// ProgramCache holds compiled programs keyed by expression source. It is safe
// for concurrent use, so requests running in parallel waves can share one.
type ProgramCache struct {
	mu    sync.RWMutex
	progs map[string]Program
}

// NewProgramCache returns an empty cache.
func NewProgramCache() *ProgramCache {
	return &ProgramCache{progs: make(map[string]Program)}
}

// Load returns the cached program for src. A nil cache holds nothing.
func (c *ProgramCache) Load(src string) (Program, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.progs[src]
	return p, ok
}

// Store caches p for src. Storing into a nil cache does nothing.
func (c *ProgramCache) Store(src string, p Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progs[src] = p
}
